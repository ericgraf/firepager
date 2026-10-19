package db

import (
	"testing"

	"github.com/firepager/backend/models"
)

func TestNormalizeAndValidateSubscriptionSettingsAcceptsCatalogChannels(t *testing.T) {
	settings := &models.SubscriptionSettings{Channels: []models.NotificationChannel{{Type: " TELEGRAM ", Address: "destination"}}}
	if err := NormalizeAndValidateSubscriptionSettings(settings); err != nil {
		t.Fatal(err)
	}
	if settings.Channels[0].Type != "telegram" {
		t.Fatalf("expected normalized channel type, got %q", settings.Channels[0].Type)
	}
}

func TestNormalizeAndValidateSubscriptionSettingsRejectsUnknownChannel(t *testing.T) {
	settings := &models.SubscriptionSettings{Channels: []models.NotificationChannel{{Type: "carrier-pigeon", Address: "destination"}}}
	if err := NormalizeAndValidateSubscriptionSettings(settings); err == nil {
		t.Fatal("expected unknown channel type to be rejected")
	}
}
