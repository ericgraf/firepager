package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/firepager/backend/db"
	"github.com/firepager/backend/models"
	"github.com/firepager/backend/notifications"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

func StartConsumer() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@firepager-rmq:5672/"
	}

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Printf("Failed to connect to RabbitMQ, retrying later: %v", err)
		return // in real scenario, we should retry loop
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("Failed to open a channel: %v", err)
		return
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"pager_messages", // name
		true,             // durable
		false,            // delete when unused
		false,            // exclusive
		false,            // no-wait
		nil,              // arguments
	)
	if err != nil {
		log.Printf("Failed to declare a queue: %v", err)
		return
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		log.Printf("Failed to register a consumer: %v", err)
		return
	}

	log.Printf("Waiting for messages from RabbitMQ...")
	for d := range msgs {
		log.Printf("Received a message: %s", d.Body)
		var msg models.Message
		if err := json.Unmarshal(d.Body, &msg); err != nil {
			log.Printf("Error unmarshalling message: %v", err)
			d.Nack(false, false) // Don't requeue invalid JSON
			continue
		}

		decodeMessage(&msg)

		if msg.ID == "" {
			msg.ID = uuid.New().String()
		}

		err := db.SaveMessage(&msg)
		if err != nil {
			log.Printf("Error saving message: %v", err)
			d.Nack(false, true) // Requeue on DB error
			continue
		}

		if msg.Type == "CALL" && msg.Address != "" {
			if err := geocodeAndStoreMessage(&msg); err != nil {
				log.Printf("Geocoding skipped for message %s: %v", msg.ID, err)
			}
		}

		d.Ack(false)
	}
}

// StartReceiverStatusConsumer consumes operational snapshots on their own
// durable queue; pager_messages remains exclusively for decoded pages.
func StartReceiverStatusConsumer() {
	for {
		runReceiverStatusConsumer()
		time.Sleep(5 * time.Second)
	}
}

func runReceiverStatusConsumer() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@firepager-rmq:5672/"
	}
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Printf("Receiver status broker connection failed: %v", err)
		return
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		log.Printf("Receiver status channel failed: %v", err)
		return
	}
	defer ch.Close()
	q, err := ch.QueueDeclare("receiver_status", true, false, false, false, nil)
	if err != nil {
		log.Printf("Receiver status queue declaration failed: %v", err)
		return
	}
	if err := ch.Qos(20, 0, false); err != nil {
		log.Printf("Receiver status QoS setup failed: %v", err)
		return
	}
	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Printf("Receiver status consumer registration failed: %v", err)
		return
	}
	log.Printf("Consuming receiver snapshots from %q", q.Name)
	for d := range msgs {
		if len(d.Body) > 32*1024 {
			d.Nack(false, false)
			continue
		}
		var status models.ReceiverStatus
		if err := json.Unmarshal(d.Body, &status); err != nil || !validReceiverStatus(&status) {
			d.Nack(false, false)
			continue
		}
		if err := db.SaveReceiverStatus(&status); err != nil {
			log.Printf("Saving receiver snapshot failed: %v", err)
			d.Nack(false, true)
			continue
		}
		d.Ack(false)
	}
}

func validReceiverStatus(s *models.ReceiverStatus) bool {
	if s.ReceiverID == "" || len(s.ReceiverID) > 100 || len(s.Host) > 255 || s.ObservedAt.IsZero() ||
		s.DecodedMessagesCount < 0 || s.CalibrationsAcceptedCount < 0 {
		return false
	}
	if time.Since(s.ObservedAt) > 24*time.Hour || s.ObservedAt.After(time.Now().Add(5*time.Minute)) {
		return false
	}
	switch s.Status {
	case "idle", "detector", "decoder", "error", "unknown", "stale":
	default:
		return false
	}
	switch s.Role {
	case "detector", "legacy", "scope", "none", "unknown":
		return true
	default:
		return false
	}
}

func geocodeAndStoreMessage(msg *models.Message) error {
	latitude, longitude, err := geocodeMessage(msg)
	if err != nil {
		return err
	}

	msg.Latitude = latitude
	msg.Longitude = longitude

	if err := db.UpdateMessageCoordinates(msg.ID, latitude, longitude); err != nil {
		return fmt.Errorf("updating coordinates in db: %w", err)
	}

	log.Printf("Stored coordinates for message %s: (%f, %f)", msg.ID, latitude, longitude)
	return nil
}

func geocodeMessage(msg *models.Message) (float64, float64, error) {
	searchQuery := buildGeocodeQuery(msg.Address, msg.Intersection)

	cityRegionContext := db.GetConfigValue("CITY_REGION_CONTEXT", "Ottawa, ON")
	if cityRegionContext != "" && !strings.Contains(strings.ToUpper(searchQuery), strings.ToUpper(cityRegionContext)) {
		searchQuery += ", " + cityRegionContext
	}

	geocodingProvider := strings.ToLower(db.GetConfigValue("GEOCODING_PROVIDER", "nominatim"))
	if geocodingProvider != "nominatim" {
		return 0, 0, fmt.Errorf("unsupported geocoding provider %q", geocodingProvider)
	}

	params := url.Values{}
	params.Set("q", searchQuery)
	params.Set("format", "jsonv2")
	params.Set("limit", "1")

	if geocodingEmail := os.Getenv("GEOCODING_EMAIL"); geocodingEmail != "" {
		params.Set("email", geocodingEmail)
	}

	apiURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?%s", params.Encode())

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("building geocode request: %w", err)
	}
	req.Header.Set("User-Agent", geocodingUserAgent())
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("requesting geocode for %q: %w", searchQuery, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("reading geocode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("geocoder returned %s: %s", resp.Status, summarizeBody(body))
	}

	var results []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}

	if err := json.Unmarshal(body, &results); err != nil {
		return 0, 0, fmt.Errorf("decoding geocode response: %w; body=%s", err, summarizeBody(body))
	}

	if len(results) > 0 {
		var latitude float64
		var longitude float64

		if _, err := fmt.Sscanf(results[0].Lat, "%f", &latitude); err != nil {
			return 0, 0, fmt.Errorf("parsing latitude %q: %w", results[0].Lat, err)
		}
		if _, err := fmt.Sscanf(results[0].Lon, "%f", &longitude); err != nil {
			return 0, 0, fmt.Errorf("parsing longitude %q: %w", results[0].Lon, err)
		}

		log.Printf("Geocoded %s to (%f, %f)", searchQuery, latitude, longitude)
		return latitude, longitude, nil
	}

	return 0, 0, fmt.Errorf("no geocoding results for %q", searchQuery)
}

func geocodingUserAgent() string {
	if custom := os.Getenv("GEOCODING_USER_AGENT"); custom != "" {
		return custom
	}

	return "FirePager-V2/1.0"
}

func buildGeocodeQuery(address, intersection string) string {
	primary := strings.TrimSpace(address)
	secondary := strings.TrimSpace(intersection)

	if looksLikeIntersection(primary) && hasStreetNumber(secondary) {
		primary, secondary = secondary, primary
	}

	if hasStreetNumber(primary) {
		return primary
	}

	if primary != "" && secondary != "" && !looksLikeIntersection(primary) && !looksLikeIntersection(secondary) {
		return primary + ", " + secondary
	}

	if primary != "" {
		return primary
	}

	return secondary
}

func hasStreetNumber(value string) bool {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return false
	}

	for _, char := range fields[0] {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}

func looksLikeIntersection(value string) bool {
	return strings.Contains(value, " & ") || strings.Contains(strings.ToLower(value), " at ")
}

func summarizeBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		return text[:200]
	}

	return text
}

// RunNotificationWorker drains the database outbox independently of the
// RabbitMQ consumer. Provider calls therefore never hold an AMQP delivery.
func RunNotificationWorker(ctx context.Context) {
	for {
		if err := ProcessOneNotification(ctx); err != nil {
			log.Printf("notification worker: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func ProcessOneNotification(ctx context.Context) error {
	delivery, err := db.ClaimNotificationDelivery(2 * time.Minute)
	if err != nil || delivery == nil {
		return err
	}
	err = notifications.Deliver(ctx, delivery.ChannelType, delivery.Address, &delivery.Message)
	if err == nil {
		return db.CompleteNotificationDelivery(delivery.ID)
	}
	// A disabled/unconfigured channel is a terminal state: retrying it forever
	// would create noise and prevent other deliveries from being claimed.
	dead := errors.Is(err, notifications.ErrProviderDisabled) || delivery.Attempts >= 5 || !retryableNotificationError(err)
	backoff := time.Duration(1<<min(delivery.Attempts, 6)) * time.Second
	if dead {
		backoff = 0
	}
	return db.FailNotificationDelivery(delivery.ID, err.Error(), time.Now().Add(backoff), dead)
}

func retryableNotificationError(err error) bool {
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "status 4") && !strings.Contains(text, "status 408") && !strings.Contains(text, "status 429") {
		return false
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func extractAllStations(vehicles []string) []string {
	return extractStations(vehicles)
}
