package db

import (
	"testing"
	"time"

	"github.com/firepager/backend/models"
)

func TestIsKnownReceiver(t *testing.T) {
	for _, id := range []string{"remote-pi-sdr", "host-sdr-0", "host-sdr-1", "host-sdr-2"} {
		if !IsKnownReceiver(id) {
			t.Errorf("expected %q to be recognized as a known receiver", id)
		}
	}

	for _, invalid := range []string{"", "   ", "unknown-receiver", "pi-sdr-999"} {
		if IsKnownReceiver(invalid) {
			t.Errorf("expected %q to be rejected as an unknown receiver", invalid)
		}
	}

	// Test SetKnownReceiverForTest
	testID := "test-custom-sdr"
	if IsKnownReceiver(testID) {
		t.Fatalf("expected %q to not be known initially", testID)
	}
	SetKnownReceiverForTest(testID, true)
	if !IsKnownReceiver(testID) {
		t.Fatalf("expected %q to be known after registration", testID)
	}
	SetKnownReceiverForTest(testID, false)
	if IsKnownReceiver(testID) {
		t.Fatalf("expected %q to be unknown after unregistration", testID)
	}
}

func TestSaveReceiverWaterfallValidation(t *testing.T) {
	// Nil payload
	if err := SaveReceiverWaterfall(nil); err == nil {
		t.Fatal("expected error on nil waterfall")
	}

	// Missing receiver ID
	w := &models.ReceiverWaterfall{
		CapturedAt: time.Now().UTC(),
	}
	if err := SaveReceiverWaterfall(w); err == nil {
		t.Fatal("expected error on missing receiver ID")
	}

	// Zero captured_at
	w.ReceiverID = "remote-pi-sdr"
	w.CapturedAt = time.Time{}
	if err := SaveReceiverWaterfall(w); err == nil {
		t.Fatal("expected error on zero captured_at")
	}

	// Nil DB
	w.CapturedAt = time.Now().UTC()
	oldDB := DB
	DB = nil
	defer func() { DB = oldDB }()

	if err := SaveReceiverWaterfall(w); err == nil {
		t.Fatal("expected error when DB is uninitialized")
	}
}

func TestGetReceiverWaterfallsValidation(t *testing.T) {
	oldDB := DB
	DB = nil
	defer func() { DB = oldDB }()

	// Empty receiver ID
	if _, err := GetReceiverWaterfalls("", 12); err == nil {
		t.Fatal("expected error on empty receiver ID")
	}

	// Valid receiver ID with nil DB returns non-nil empty slice
	items, err := GetReceiverWaterfalls("remote-pi-sdr", 12)
	if err != nil {
		t.Fatalf("unexpected error with nil DB: %v", err)
	}
	if items == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}
