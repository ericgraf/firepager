package models

import "time"

// ReceiverStatus is an operational snapshot; it is deliberately separate from
// pager Message traffic and may be overwritten by the next heartbeat.
type ReceiverStatus struct {
	ReceiverID                string     `json:"receiver_id"`
	Host                      string     `json:"host"`
	Status                    string     `json:"status"`
	Role                      string     `json:"role"`
	FrequencyHz               *float64   `json:"frequency_hz"`
	GainDB                    *float64   `json:"gain_db"`
	PPM                       *int       `json:"ppm"`
	ActiveDetector            bool       `json:"active_detector"`
	DecodedMessagesCount      int64      `json:"decoded_messages_count"`
	CalibrationsAcceptedCount int64      `json:"calibrations_accepted_count"`
	LastCalibrationUTC        *time.Time `json:"last_calibration_utc"`
	ObservedAt                time.Time  `json:"observed_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}
