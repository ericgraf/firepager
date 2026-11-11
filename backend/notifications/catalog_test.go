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

func TestBaileysCatalogUsesSecretFromEnvironment(t *testing.T) {
	t.Setenv("WHATSAPP_BRIDGE_AUTH_TOKEN", "test-token")
	t.Setenv("BRIDGE_ALLOWLIST", "+15555550123")
	catalog := BuildChannelCatalog(map[string]string{
		"WHATSAPP_GATEWAY_ENABLED": "true",
		"WHATSAPP_PROVIDER":        "baileys",
		"WHATSAPP_BRIDGE_URL":      "http://whatsapp-bridge:8787",
		"WHATSAPP_VERIFIED":        "true",
	}, &models.User{PhoneNumber: "+15555550123"})
	for _, channel := range catalog {
		if channel.ID != "whatsapp" {
			continue
		}
		if !channel.Configured || !channel.DeliveryImplemented || !channel.Usable {
			t.Fatalf("expected configured Baileys WhatsApp channel to be usable: %+v", channel)
		}
		return
	}
	t.Fatal("WhatsApp channel missing from catalog")
}

func TestBaileysCatalogSupportsSharedSenderForOtherUsers(t *testing.T) {
	t.Setenv("WHATSAPP_BRIDGE_AUTH_TOKEN", "test-token")
	t.Setenv("BRIDGE_ALLOWLIST", "+15555550123")
	catalog := BuildChannelCatalog(map[string]string{
		"WHATSAPP_GATEWAY_ENABLED": "true",
		"WHATSAPP_PROVIDER":        "baileys",
		"WHATSAPP_BRIDGE_URL":      "http://whatsapp-bridge:8787",
	}, &models.User{PhoneNumber: "+15555550124"})
	for _, channel := range catalog {
		if channel.ID == "whatsapp" {
			if !channel.Configured {
				t.Fatalf("shared WhatsApp sender should be available to other users: %+v", channel)
			}
			return
		}
	}
	t.Fatal("WhatsApp channel missing from catalog")
}
