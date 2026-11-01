package mq

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/firepager/backend/db"
	"github.com/firepager/backend/models"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// WaterfallQueueName is the durable RabbitMQ queue for waterfall snapshots.
	WaterfallQueueName = "receiver_waterfalls"

	// MaxWaterfallPayloadBytes is the maximum allowed size for a single snapshot (1 MiB).
	MaxWaterfallPayloadBytes = 1024 * 1024

	// MaxWaterfallRows is the maximum number of time-slice rows in a snapshot.
	MaxWaterfallRows = 120

	// MaxWaterfallBinsPerRow is the maximum number of frequency bins per row.
	MaxWaterfallBinsPerRow = 512
)

// ValidateWaterfallPayload enforces size limits, JSON schema validity, and domain constraints.
func ValidateWaterfallPayload(body []byte) (*models.ReceiverWaterfall, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	if len(body) > MaxWaterfallPayloadBytes {
		return nil, fmt.Errorf("payload exceeds 1 MiB limit (%d bytes)", len(body))
	}

	var w models.ReceiverWaterfall
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("unmarshaling waterfall json: %w", err)
	}

	if err := ValidateReceiverWaterfall(&w); err != nil {
		return nil, err
	}

	return &w, nil
}

// ValidateReceiverWaterfall checks bounds, finite numbers, timestamp sanity, and known receiver ID.
func ValidateReceiverWaterfall(w *models.ReceiverWaterfall) error {
	if w == nil {
		return fmt.Errorf("waterfall cannot be nil")
	}

	if !db.IsKnownReceiver(w.ReceiverID) {
		return fmt.Errorf("unknown receiver_id %q", w.ReceiverID)
	}

	if w.CapturedAt.IsZero() {
		return fmt.Errorf("captured_at is required")
	}
	if w.CapturedAt.After(time.Now().Add(5 * time.Minute)) {
		return fmt.Errorf("captured_at is in the future: %v", w.CapturedAt)
	}
	if time.Since(w.CapturedAt) > db.DefaultWaterfallRetention {
		return fmt.Errorf("captured_at is older than retention window (%v): %v", db.DefaultWaterfallRetention, w.CapturedAt)
	}

	if math.IsNaN(w.StartHz) || math.IsInf(w.StartHz, 0) || w.StartHz <= 0 {
		return fmt.Errorf("start_hz must be a positive finite number, got %v", w.StartHz)
	}
	if math.IsNaN(w.StepHz) || math.IsInf(w.StepHz, 0) || w.StepHz <= 0 {
		return fmt.Errorf("step_hz must be a positive finite number, got %v", w.StepHz)
	}

	if len(w.Rows) == 0 {
		return fmt.Errorf("waterfall must contain at least 1 row")
	}
	if len(w.Rows) > MaxWaterfallRows {
		return fmt.Errorf("waterfall exceeds max %d rows (got %d)", MaxWaterfallRows, len(w.Rows))
	}

	for i, row := range w.Rows {
		if row.CapturedAt.IsZero() {
			return fmt.Errorf("row %d has zero captured_at", i)
		}
		if row.CapturedAt.After(w.CapturedAt) {
			return fmt.Errorf("row %d captured_at is later than snapshot captured_at", i)
		}
		if i > 0 && row.CapturedAt.Before(w.Rows[i-1].CapturedAt) {
			return fmt.Errorf("row %d timestamp precedes the previous row", i)
		}
		if len(row.PowerDB) == 0 {
			return fmt.Errorf("row %d has 0 power_db bins", i)
		}
		if len(row.PowerDB) > MaxWaterfallBinsPerRow {
			return fmt.Errorf("row %d exceeds max %d bins (got %d)", i, MaxWaterfallBinsPerRow, len(row.PowerDB))
		}
		for binIdx, p := range row.PowerDB {
			if math.IsNaN(p) || math.IsInf(p, 0) {
				return fmt.Errorf("row %d bin %d value is not finite: %v", i, binIdx, p)
			}
		}
	}

	return nil
}

// StartReceiverWaterfallConsumer starts the persistent consumer loop with automatic reconnection.
func StartReceiverWaterfallConsumer() {
	for {
		runReceiverWaterfallConsumer()
		time.Sleep(5 * time.Second)
	}
}

func runReceiverWaterfallConsumer() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@firepager-rmq:5672/"
	}

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Printf("Receiver waterfall broker connection failed: %v", err)
		return
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("Receiver waterfall channel failed: %v", err)
		return
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		WaterfallQueueName,
		true,  // durable (publisher-ack compatible)
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Printf("Receiver waterfall queue declaration failed: %v", err)
		return
	}

	if err := ch.Qos(10, 0, false); err != nil {
		log.Printf("Receiver waterfall QoS setup failed: %v", err)
		return
	}

	msgs, err := ch.Consume(
		q.Name,
		"",    // consumer tag
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Printf("Receiver waterfall consumer registration failed: %v", err)
		return
	}

	log.Printf("Consuming receiver waterfall snapshots from %q", q.Name)
	for d := range msgs {
		waterfall, err := ValidateWaterfallPayload(d.Body)
		if err != nil {
			log.Printf("Invalid receiver waterfall snapshot rejected: %v", err)
			d.Nack(false, false) // reject without requeue
			continue
		}

		if err := db.SaveReceiverWaterfall(waterfall); err != nil {
			log.Printf("Saving receiver waterfall failed: %v", err)
			d.Nack(false, true) // requeue on DB error
			continue
		}

		d.Ack(false) // ack only after DB commit
	}
}
