package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReceiverWaterfallJSONContract(t *testing.T) {
	rawJSON := `{
		"receiver_id": "remote-pi-sdr",
		"captured_at": "2026-10-31T14:30:00Z",
		"start_hz": 155538000,
		"step_hz": 250,
		"rows": [
			{
				"captured_at": "2026-10-31T14:29:55Z",
				"power_db": [-45.5, -46.2, -44.1]
			},
			{
				"captured_at": "2026-10-31T14:30:00Z",
				"power_db": [-43.0, -42.8, -43.5]
			}
		]
	}`

	var waterfall ReceiverWaterfall
	if err := json.Unmarshal([]byte(rawJSON), &waterfall); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if waterfall.ReceiverID != "remote-pi-sdr" {
		t.Errorf("expected receiver_id 'remote-pi-sdr', got %s", waterfall.ReceiverID)
	}
	if waterfall.StartHz != 155538000 {
		t.Errorf("expected start_hz 155538000, got %f", waterfall.StartHz)
	}
	if waterfall.StepHz != 250 {
		t.Errorf("expected step_hz 250, got %f", waterfall.StepHz)
	}
	if len(waterfall.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(waterfall.Rows))
	}
	if len(waterfall.Rows[0].PowerDB) != 3 {
		t.Errorf("expected 3 power_db bins in row 0, got %d", len(waterfall.Rows[0].PowerDB))
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2026-10-31T14:30:00Z")
	if !waterfall.CapturedAt.Equal(expectedTime) {
		t.Errorf("expected captured_at %v, got %v", expectedTime, waterfall.CapturedAt)
	}

	marshaled, err := json.Marshal(waterfall)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var roundtrip map[string]interface{}
	if err := json.Unmarshal(marshaled, &roundtrip); err != nil {
		t.Fatalf("unmarshal roundtrip error: %v", err)
	}

	for _, key := range []string{"receiver_id", "captured_at", "start_hz", "step_hz", "rows"} {
		if _, ok := roundtrip[key]; !ok {
			t.Errorf("missing key %q in marshaled JSON", key)
		}
	}
}
