package mq

import (
	"encoding/json"
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

		checkAlerts(&msg)
		d.Ack(false)
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

func checkAlerts(msg *models.Message) {
	users, err := db.GetUsers()
	if err != nil {
		log.Printf("Error getting users for alerts: %v", err)
		return
	}

	for _, user := range users {
		match := false

		// Match Capcode
		for _, c := range user.Capcodes {
			if c == msg.Source {
				match = true
				break
			}
		}

		if !match && msg.Type == "CALL" {
			// Match Vehicle
			for _, vSub := range user.Vehicles {
				for _, vInMsg := range msg.Vehicles {
					if strings.EqualFold(vSub, vInMsg) {
						match = true
						break
					}
				}
				if match {
					break
				}
			}

			// Match Station
			if !match {
				// We should check all stations for all vehicles in the call
				stations := extractAllStations(msg.Vehicles)
				for _, sSub := range user.Stations {
					for _, sInMsg := range stations {
						if sSub == sInMsg {
							match = true
							break
						}
					}
					if match {
						break
					}
				}
			}

			// Match Call Type
			if !match {
				for _, ctSub := range user.CallTypes {
					if strings.Contains(strings.ToUpper(msg.CallType), strings.ToUpper(ctSub)) {
						match = true
						break
					}
				}
			}
		}

		if match {
			log.Printf("ALERT: User %s notified for message %s", user.Email, msg.ID)
			notifications.SendNotification(&user, msg)
		}
	}
}

func extractAllStations(vehicles []string) []string {
	return extractStations(vehicles)
}
