package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/firepager/backend/db"
	"github.com/firepager/backend/models"
)

// SendNotification sends alerts to a user based on their preferences
func SendNotification(user *models.User, msg *models.Message) {
	if len(user.NotificationTypes) == 0 {
		log.Printf("User %s (%s) has no notification types configured", user.Name, user.Email)
		return
	}

	for _, nType := range user.NotificationTypes {
		switch strings.ToUpper(nType) {
		case "SMS":
			sendSMS(user, msg)
		case "EMAIL":
			sendEmail(user, msg)
		case "PUSH":
			sendPush(user, msg)
		case "WEB_PUSH", "WEBPUSH":
			sendConfiguredProvider("web_push", user.Email, user, msg)
		case "TELEGRAM", "SIGNAL", "NTFY", "DISCORD", "SLACK", "TEAMS", "VOICE":
			sendConfiguredProvider(strings.ToLower(nType), user.Email, user, msg)
		case "PUSHOVER":
			sendPushover(user, msg)
		case "WHATSAPP":
			sendWhatsApp(user, msg)
		default:
			log.Printf("Unknown notification type: %s for user %s", nType, user.Email)
		}
	}
}

func sendSMS(user *models.User, msg *models.Message) {
	if user.PhoneNumber == "" {
		log.Printf("SMS requested for user %s, but no phone number provided", user.Email)
		return
	}

	if !configEnabled("SMS_GATEWAY_ENABLED") {
		log.Printf("SMS requested for user %s, but SMS gateway is disabled", user.Email)
		return
	}

	provider := strings.ToLower(db.GetConfigValue("SMS_PROVIDER", "twilio"))
	if provider != "twilio" {
		log.Printf("SMS provider %q is not supported for user %s", provider, user.Email)
		return
	}

	accountSID := strings.TrimSpace(db.GetConfigValue("TWILIO_ACCOUNT_SID", ""))
	authToken := strings.TrimSpace(db.GetConfigValue("TWILIO_AUTH_TOKEN", ""))
	fromNumber := strings.TrimSpace(db.GetConfigValue("TWILIO_FROM_NUMBER", ""))
	if accountSID == "" || authToken == "" || fromNumber == "" {
		log.Printf("Twilio SMS requested for user %s, but Twilio configuration is incomplete", user.Email)
		return
	}

	content := buildSMSContent(msg)
	if err := sendTwilioMessage(accountSID, authToken, normalizePhoneNumber(user.PhoneNumber), fromNumber, content); err != nil {
		log.Printf("Twilio SMS failed for %s: %v", user.Email, err)
		return
	}

	log.Printf("Twilio SMS sent successfully to %s", user.PhoneNumber)
}

func sendEmail(user *models.User, msg *models.Message) {
	if user.Email == "" {
		return
	}
	sendConfiguredProvider("email", user.Email, user, msg)
}

func sendPush(user *models.User, msg *models.Message) {
	// "push" is retained for existing subscriptions; actual Web Push requires
	// a provider endpoint and a registered subscription token.
	sendConfiguredProvider("push", user.Email, user, msg)
}

func sendConfiguredProvider(name, recipient string, user *models.User, msg *models.Message) {
	configs, err := db.GetConfigs()
	if err != nil {
		log.Printf("%s notification skipped: provider configuration unavailable", name)
		return
	}
	values := make(map[string]string, len(configs))
	for _, config := range configs {
		values[config.Key] = config.Value
	}
	cfg := ProviderConfigFromMap(values, name)
	if !cfg.Usable() {
		log.Printf("%s notification skipped: provider is not enabled, verified, and configured", name)
		return
	}
	provider := NewHTTPProvider(cfg)
	request := NotificationRequest{
		Provider: name, Recipient: recipient,
		Subject:  fmt.Sprintf("Firepager Alert: %s #%s", msg.CallType, msg.IncidentNumber),
		Body:     buildNotificationContent(msg),
		Metadata: map[string]string{"user": user.Email},
	}
	if err := provider.Send(context.Background(), request); err != nil {
		log.Printf("%s notification failed: %v", name, err)
	}
}

func buildNotificationContent(msg *models.Message) string {
	return fmt.Sprintf("Alert\nIncident: %s\nAddress: %s\nVehicles: %s\nStation: %s\n\n%s",
		msg.IncidentNumber, msg.Address, formatVehicles(msg.Vehicles), msg.Station, msg.Text)
}

func sendPushover(user *models.User, msg *models.Message) {
	if user.PushoverUserKey == "" || user.PushoverAppToken == "" {
		log.Printf("Pushover requested for user %s, but keys are missing", user.Email)
		return
	}

	title := fmt.Sprintf("[%s] %s", msg.CallType, msg.IncidentNumber)
	message := fmt.Sprintf("%s\nVehicles: %s\nStation: %s\n\n%s", msg.Address, formatVehicles(msg.Vehicles), msg.Station, msg.Text)

	formData := url.Values{
		"token":   {user.PushoverAppToken},
		"user":    {user.PushoverUserKey},
		"title":   {title},
		"message": {message},
	}

	resp, err := http.PostForm("https://api.pushover.net/1/messages.json", formData)
	if err != nil {
		log.Printf("Error sending Pushover notification for %s: %v", user.Email, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Pushover API returned non-OK status for %s: %s", user.Email, resp.Status)
	} else {
		log.Printf("Pushover notification sent successfully to %s", user.Email)
	}
}

func sendWhatsApp(user *models.User, msg *models.Message) {
	// The legacy Twilio path is intentionally inert. WhatsApp delivery must be
	// wired through WhatsAppAdapter with an explicit provider and consent gate;
	// this notification entry point has neither a consent record nor a provider.
	log.Printf("WhatsApp requested, but WhatsApp delivery is not wired")
}

func configEnabled(key string) bool {
	value := strings.TrimSpace(strings.ToLower(db.GetConfigValue(key, "false")))
	return value == "true" || value == "1" || value == "yes" || value == "on"
}

func sendTwilioMessage(accountSID, authToken, to, from, body string) error {
	formData := url.Values{
		"To":   {to},
		"From": {from},
		"Body": {body},
	}

	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", accountSID)
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("creating Twilio request: %w", err)
	}

	req.SetBasicAuth(accountSID, authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s", summarizeAPIError(responseBody))
	}

	return nil
}

func buildSMSContent(msg *models.Message) string {
	var parts []string

	if msg.CallType != "" {
		parts = append(parts, fmt.Sprintf("[%s]", msg.CallType))
	}
	if msg.IncidentNumber != "" {
		parts = append(parts, "#"+msg.IncidentNumber)
	}
	if msg.Address != "" {
		parts = append(parts, "@ "+msg.Address)
	}
	if vehicles := formatVehicles(msg.Vehicles); vehicles != "" {
		parts = append(parts, "Vehicles: "+vehicles)
	}
	if msg.Station != "" {
		parts = append(parts, "Station: "+msg.Station)
	}

	content := "FIREPAGER ALERT"
	if len(parts) > 0 {
		content += " " + strings.Join(parts, " ")
	}

	if len(content) > 320 {
		return content[:317] + "..."
	}

	return content
}

func buildWhatsAppContent(msg *models.Message) string {
	content := buildSMSContent(msg)

	if msg.Text != "" {
		content += "\n\n" + msg.Text
	}

	if len(content) > 1200 {
		return content[:1197] + "..."
	}

	return content
}

func normalizePhoneNumber(phone string) string {
	return strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '+' {
			return r
		}
		return -1
	}, phone)
}

func summarizeAPIError(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}

	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err == nil && strings.TrimSpace(payload.Message) != "" {
		return payload.Message
	}

	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		return text[:200]
	}
	if text == "" {
		return "empty response body"
	}
	return text
}

func formatVehicles(vehicles []string) string {
	if len(vehicles) == 0 {
		return ""
	}

	return strings.Join(vehicles, ", ")
}
