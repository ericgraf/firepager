package notifications

import (
	"testing"

	"github.com/firepager/backend/models"
)

func TestBuildChannelCatalogRequiresVerificationAndImplementation(t *testing.T) {
	config := map[string]string{
		"SMS_GATEWAY_ENABLED": "true",
		"SMS_PROVIDER":        "twilio",
		"TWILIO_ACCOUNT_SID":  "account",
		"TWILIO_AUTH_TOKEN":   "secret",
		"TWILIO_FROM_NUMBER":  "+15550000000",
		"SMS_VERIFIED":        "false",
		"TELEGRAM_ENABLED":    "true",
		"TELEGRAM_TOKEN":      "token",
		"TELEGRAM_VERIFIED":   "true",
	}

	catalog := BuildChannelCatalog(config, &models.User{Email: "user@example.com"})
	byID := make(map[string]models.NotificationChannelCatalog, len(catalog))
	for _, channel := range catalog {
		byID[channel.ID] = channel
	}

	if byID["sms"].Configured != true || byID["sms"].Verified || byID["sms"].Usable {
		t.Fatalf("expected configured but unverified SMS to be unusable: %+v", byID["sms"])
	}
	if !byID["telegram"].Configured || !byID["telegram"].Verified || byID["telegram"].Usable {
		t.Fatalf("expected unimplemented Telegram to remain unusable: %+v", byID["telegram"])
	}
	for _, id := range []string{"sms", "email", "whatsapp", "telegram", "signal", "pushover", "ntfy", "web_push", "discord", "slack", "teams", "voice"} {
		if !byID[id].Supported {
			t.Errorf("expected %s in supported channel catalog", id)
		}
	}
}

func TestBuildChannelCatalogDoesNotClaimDisabledSMS(t *testing.T) {
	catalog := BuildChannelCatalog(map[string]string{
		"SMS_GATEWAY_ENABLED": "false",
		"SMS_VERIFIED":        "true",
	}, &models.User{})
	if !catalog[0].Disabled || catalog[0].Usable {
		t.Fatalf("expected disabled SMS to be unusable: %+v", catalog[0])
	}
}
