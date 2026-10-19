package mq

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/firepager/backend/models"
)

func TestMessageDecoding(t *testing.T) {
	samples := []struct {
		name     string
		rawJSON  string
		expected models.Message
	}{
		{
			name: "Sample 1: SMOKE VISIBLE",
			rawJSON: `{
				"client_id": "pi-1",
				"timestamp": "2026-03-22T15:55:48Z",
				"raw_text": "POCSAG512: Address:  930032  Function: 2  Alpha:   ID:gdsJ3D<LF>SMOKE VISIBLE INSIDE-FIRE <LF>15:55:48<LF>6525 BANK ST  Dv: 9  Sec: 91D <LF>Gd: 91D<LF>CENOTE RD & STONE SCHOOL RD<LF>PT91 PT93 P92 LA91 TA92 TA94<LF>TA83 SC90 #36719<LF>Sent by: FDP2<EOT><NUL>"
			}`,
			expected: models.Message{
				Type:           "CALL",
				PageID:         "gdsJ3D",
				CallType:       "SMOKE VISIBLE INSIDE-FIRE",
				IncidentNumber: "36719",
				Vehicles:       []string{"PT91", "PT93", "P92", "LA91", "TA92", "TA94", "TA83", "SC90"},
				Address:        "6525 BANK ST",
				Intersection:   "CENOTE RD & STONE SCHOOL RD PT91 PT93 P92 LA91 TA92 TA94",
				Source:         "930032",
				Station:        "91,93,92,94,83,90",
			},
		},
		{
			name: "Sample 2: MVC-ROLLOVER",
			rawJSON: `{
				"client_id": "pi-2",
				"timestamp": "2026-03-22T16:35:53Z",
				"raw_text": "POCSAG512: Address:    1296  Function: 2  Alpha:   ID:0VFZuQ<LF>MVC-ROLLOVER  16:35:53<LF>3168 DALMAC RD  Dv: 9  Sec: 92B<LF> Gd: 92B<LF>FOREST RD & SPRINGHILL RD<LF>P92 RE93 SO23 TA92 SC90 #37196<LF>Sent by: FDP1<EOT>"
			}`,
			expected: models.Message{
				Type:           "CALL",
				PageID:         "0VFZuQ",
				CallType:       "MVC-ROLLOVER",
				IncidentNumber: "37196",
				Vehicles:       []string{"P92", "RE93", "SO23", "TA92", "SC90"},
				Address:        "3168 DALMAC RD",
				Intersection:   "FOREST RD & SPRINGHILL RD",
				Source:         "1296",
				Station:        "92,93,23,90",
			},
		},
		{
			name: "Sample 3: multi-station dispatch preserves unique order",
			rawJSON: `{
				"client_id": "pi-3",
				"timestamp": "2026-03-22T17:01:00Z",
				"raw_text": "POCSAG512: Address:  930032  Function: 2  Alpha:   ID:test01<LF>WORKING FIRE<LF>17:01:00<LF>100 MAIN ST  Dv: 9  Sec: 91D <LF>Gd: 91D<LF>FIRST ST & SECOND ST<LF>PT91 TA83 PT91 RE101 #40001<LF>Sent by: FDP2<EOT><NUL>"
			}`,
			expected: models.Message{
				Type:           "CALL",
				PageID:         "test01",
				CallType:       "WORKING FIRE",
				IncidentNumber: "40001",
				Vehicles:       []string{"PT91", "TA83", "PT91", "RE101"},
				Address:        "100 MAIN ST",
				Intersection:   "FIRST ST & SECOND ST",
				Source:         "930032",
				Station:        "91,83,10",
			},
		},
	}

	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			var msg models.Message
			err := json.Unmarshal([]byte(s.rawJSON), &msg)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			decodeMessage(&msg)

			if msg.Type != s.expected.Type {
				t.Errorf("Expected type %s, got %s", s.expected.Type, msg.Type)
			}
			if msg.PageID != s.expected.PageID {
				t.Errorf("Expected PageID %s, got %s", s.expected.PageID, msg.PageID)
			}
			if msg.CallType != s.expected.CallType {
				t.Errorf("Expected CallType %s, got %s", s.expected.CallType, msg.CallType)
			}
			if msg.IncidentNumber != s.expected.IncidentNumber {
				t.Errorf("Expected IncidentNumber %s, got %s", s.expected.IncidentNumber, msg.IncidentNumber)
			}
			if len(msg.Vehicles) != len(s.expected.Vehicles) {
				t.Fatalf("Expected vehicles %v, got %v", s.expected.Vehicles, msg.Vehicles)
			}
			for i := range s.expected.Vehicles {
				if msg.Vehicles[i] != s.expected.Vehicles[i] {
					t.Fatalf("Expected vehicles %v, got %v", s.expected.Vehicles, msg.Vehicles)
				}
			}
			if msg.Address != s.expected.Address {
				t.Errorf("Expected Address %s, got %s", s.expected.Address, msg.Address)
			}
			if msg.Source != s.expected.Source {
				t.Errorf("Expected source %s, got %s", s.expected.Source, msg.Source)
			}
			if msg.Station != s.expected.Station {
				t.Errorf("Expected station %s, got %s", s.expected.Station, msg.Station)
			}
		})
	}
}

func TestNotificationRetryClassification(t *testing.T) {
	if retryableNotificationError(fmt.Errorf("email provider request failed (status 503)")) == false {
		t.Fatal("expected server failures to retry")
	}
	if retryableNotificationError(fmt.Errorf("email provider request failed (status 400)")) {
		t.Fatal("did not expect permanent client failures to retry")
	}
	if min(8, 6) != 6 {
		t.Fatal("backoff cap helper is incorrect")
	}
}
