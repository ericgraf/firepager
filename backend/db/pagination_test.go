package db

import (
	"testing"
	"time"

	"github.com/firepager/backend/models"
)

func TestMessageCursorRoundTrip(t *testing.T) {
	original := models.Message{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		Timestamp: time.Date(2026, 10, 19, 12, 34, 56, 123456789, time.FixedZone("EDT", -4*60*60)),
	}
	cursor := encodeMessageCursor(original)
	gotTime, gotID, err := decodeMessageCursor(cursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if !gotTime.Equal(original.Timestamp) || gotID != original.ID {
		t.Fatalf("cursor decoded as %s/%s, want %s/%s", gotTime, gotID, original.Timestamp, original.ID)
	}
}

func TestDecodeMessageCursorRejectsMalformedValue(t *testing.T) {
	if _, _, err := decodeMessageCursor("not-a-cursor"); err == nil {
		t.Fatal("expected malformed cursor to be rejected")
	}
}
