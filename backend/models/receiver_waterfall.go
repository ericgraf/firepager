package models

import "time"

// WaterfallRow represents a single time-slice row in a waterfall snapshot.
type WaterfallRow struct {
	CapturedAt time.Time `json:"captured_at"`
	PowerDB    []float64 `json:"power_db"`
}

// ReceiverWaterfall represents a periodic waterfall snapshot payload and stored record.
type ReceiverWaterfall struct {
	ID         int64          `json:"id,omitempty"`
	ReceiverID string         `json:"receiver_id"`
	CapturedAt time.Time      `json:"captured_at"`
	StartHz    float64        `json:"start_hz"`
	StepHz     float64        `json:"step_hz"`
	Rows       []WaterfallRow `json:"rows"`
	CreatedAt  time.Time      `json:"created_at,omitempty"`
}
