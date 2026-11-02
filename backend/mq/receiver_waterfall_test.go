package mq

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/firepager/backend/models"
)

func validTestWaterfall() models.ReceiverWaterfall {
	now := time.Now().UTC()
	return models.ReceiverWaterfall{
		ReceiverID: "remote-pi-sdr",
		CapturedAt: now,
		StartHz:    155538000,
		StepHz:     250,
		Rows: []models.WaterfallRow{
			{
				CapturedAt: now.Add(-5 * time.Second),
				PowerDB:    []float64{-45.5, -46.2, -44.1},
			},
			{
				CapturedAt: now,
				PowerDB:    []float64{-43.0, -42.8, -43.5},
			},
		},
	}
}

func TestValidateReceiverWaterfall_Valid(t *testing.T) {
	w := validTestWaterfall()
	if err := ValidateReceiverWaterfall(&w); err != nil {
		t.Fatalf("expected valid waterfall to pass, got: %v", err)
	}
}

func TestValidateReceiverWaterfall_Nil(t *testing.T) {
	if err := ValidateReceiverWaterfall(nil); err == nil {
		t.Fatal("expected error on nil waterfall")
	}
}

func TestValidateReceiverWaterfall_UnknownReceiver(t *testing.T) {
	w := validTestWaterfall()
	w.ReceiverID = "unknown-sdr-device"
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on unknown receiver ID")
	}

	w.ReceiverID = ""
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on empty receiver ID")
	}
}

func TestValidateReceiverWaterfall_TimestampChecks(t *testing.T) {
	w := validTestWaterfall()
	w.CapturedAt = time.Time{}
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on zero captured_at")
	}

	// Future timestamp > 5 minutes
	w = validTestWaterfall()
	w.CapturedAt = time.Now().Add(10 * time.Minute)
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on future captured_at")
	}

	// Stale timestamp > 48 hours
	w = validTestWaterfall()
	w.CapturedAt = time.Now().Add(-50 * time.Hour)
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on stale captured_at")
	}
}

func TestValidateReceiverWaterfall_FrequencyMetadata(t *testing.T) {
	// StartHz <= 0
	w := validTestWaterfall()
	w.StartHz = 0
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on zero StartHz")
	}

	// StartHz NaN
	w = validTestWaterfall()
	w.StartHz = math.NaN()
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on NaN StartHz")
	}

	// StartHz Inf
	w = validTestWaterfall()
	w.StartHz = math.Inf(1)
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on Inf StartHz")
	}

	// StepHz <= 0
	w = validTestWaterfall()
	w.StepHz = -10
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on negative StepHz")
	}

	// StepHz NaN
	w = validTestWaterfall()
	w.StepHz = math.NaN()
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on NaN StepHz")
	}
}

func TestValidateReceiverWaterfall_RowsBounds(t *testing.T) {
	// 0 rows
	w := validTestWaterfall()
	w.Rows = []models.WaterfallRow{}
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on empty rows")
	}

	// > 120 rows
	w = validTestWaterfall()
	rows := make([]models.WaterfallRow, 121)
	now := time.Now().UTC()
	for i := range rows {
		rows[i] = models.WaterfallRow{
			CapturedAt: now.Add(-time.Duration(120-i) * time.Second),
			PowerDB:    []float64{-50.0},
		}
	}
	w.Rows = rows
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error when rows exceed 120")
	}

	// Exactly 120 rows should pass
	w.Rows = rows[:120]
	if err := ValidateReceiverWaterfall(&w); err != nil {
		t.Fatalf("expected 120 rows to pass, got: %v", err)
	}
}

func TestValidateReceiverWaterfall_RowBinsBounds(t *testing.T) {
	// Zero row captured_at
	w := validTestWaterfall()
	w.Rows[0].CapturedAt = time.Time{}
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on zero row captured_at")
	}

	// 0 bins in row
	w = validTestWaterfall()
	w.Rows[0].PowerDB = []float64{}
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on 0 power_db bins")
	}

	// > 512 bins in row
	w = validTestWaterfall()
	w.Rows[0].PowerDB = make([]float64, 513)
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error when bins exceed 512")
	}

	// Exactly 512 bins should pass
	w.Rows[0].PowerDB = make([]float64, 512)
	for i := range w.Rows[0].PowerDB {
		w.Rows[0].PowerDB[i] = -40.0
	}
	if err := ValidateReceiverWaterfall(&w); err != nil {
		t.Fatalf("expected 512 bins to pass, got: %v", err)
	}

	// Non-finite bin value (NaN)
	w = validTestWaterfall()
	w.Rows[0].PowerDB = []float64{-45.0, math.NaN(), -46.0}
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on NaN bin value")
	}

	// Non-finite bin value (Inf)
	w = validTestWaterfall()
	w.Rows[0].PowerDB = []float64{-45.0, math.Inf(-1), -46.0}
	if err := ValidateReceiverWaterfall(&w); err == nil {
		t.Fatal("expected error on -Inf bin value")
	}
}

func TestValidateWaterfallPayload(t *testing.T) {
	// Empty payload
	if _, err := ValidateWaterfallPayload(nil); err == nil {
		t.Fatal("expected error on nil payload")
	}
	if _, err := ValidateWaterfallPayload([]byte{}); err == nil {
		t.Fatal("expected error on empty payload")
	}

	// Exceeds 1 MiB
	oversized := make([]byte, MaxWaterfallPayloadBytes+1)
	if _, err := ValidateWaterfallPayload(oversized); err == nil {
		t.Fatal("expected error on payload exceeding 1 MiB")
	}

	// Malformed JSON
	if _, err := ValidateWaterfallPayload([]byte("not json")); err == nil {
		t.Fatal("expected error on malformed JSON")
	}

	// Valid payload
	w := validTestWaterfall()
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	parsed, err := ValidateWaterfallPayload(data)
	if err != nil {
		t.Fatalf("expected valid payload to pass, got: %v", err)
	}
	if parsed.ReceiverID != w.ReceiverID {
		t.Errorf("expected receiver %s, got %s", w.ReceiverID, parsed.ReceiverID)
	}

	// JSON with whitespace padding up to 1 MiB limit boundary
	padding := bytes.Repeat([]byte(" "), 1000)
	paddedData := append(data, padding...)
	if _, err := ValidateWaterfallPayload(paddedData); err != nil {
		t.Fatalf("expected padded valid json to pass, got: %v", err)
	}
}
