package mq

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/firepager/backend/models"
)

func TestReceiverStatusValidation(t *testing.T) {
	valid := models.ReceiverStatus{
		ReceiverID: "host-sdr-0", Host: "localhost", Status: "idle", Role: "legacy",
		ObservedAt: time.Now().UTC(),
	}
	if !validReceiverStatus(&valid) {
		t.Fatal("expected valid receiver snapshot to pass")
	}
	valid.ReceiverID = ""
	if validReceiverStatus(&valid) {
		t.Fatal("expected snapshot without receiver_id to fail")
	}
	valid.ReceiverID = "host-sdr-0"
	valid.ObservedAt = time.Now().Add(-48 * time.Hour)
	if validReceiverStatus(&valid) {
		t.Fatal("expected stale snapshot to fail ingestion validation")
	}
}

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
				IncidentNumber: "26-36719",
				Vehicles:       []string{"PT91", "PT93", "P92", "LA91", "TA92", "TA94", "TA83", "SC90"},
				Address:        "6525 BANK ST",
				Intersection:   "CENOTE RD & STONE SCHOOL RD",
				DV:             "9", Section: "91D", Grid: "91D",
				Source:   "930032",
				Stations: []string{"91", "93", "92", "94", "83", "90"},
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
				IncidentNumber: "26-37196",
				Vehicles:       []string{"P92", "RE93", "SO23", "TA92", "SC90"},
				Address:        "3168 DALMAC RD",
				Intersection:   "FOREST RD & SPRINGHILL RD",
				DV:             "9", Section: "92B", Grid: "92B",
				Source:   "1296",
				Stations: []string{"92", "93", "23", "90"},
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
				IncidentNumber: "26-40001",
				Vehicles:       []string{"PT91", "TA83", "PT91", "RE101"},
				Address:        "100 MAIN ST",
				Intersection:   "FIRST ST & SECOND ST",
				DV:             "9", Section: "91D", Grid: "91D",
				Source:   "930032",
				Stations: []string{"91", "83", "10"},
			},
		},
		{
			name:    "Flattened page 114676: smoke inside-fire",
			rawJSON: `{"raw_text":"POCSAG512: Address:    1136  Function: 2  Alpha:   SMOKE VISIBLE INSIDE-FIRE  16:52:23 8472 FRANKTOWN RD  Dv: 8  Sec: 82G  Gd: V33 SKEEL CRT & DWYER HILL RD/Alias:10 HWY P82 P81 P46 T46 TA82 TA81 PT81 SC60 #114676 Sent by: FDP1<NUL>"}`,
			expected: models.Message{
				Type: "CALL", CallType: "SMOKE VISIBLE INSIDE-FIRE", IncidentNumber: "26-114676",
				Vehicles: []string{"P82", "P81", "P46", "T46", "TA82", "TA81", "PT81", "SC60"},
				Address:  "8472 FRANKTOWN RD", Intersection: "SKEEL CRT & DWYER HILL RD",
				DV: "8", Section: "82G", Grid: "V33", Alias: "10 HWY",
				Source: "1136", Stations: []string{"82", "81", "46", "60"},
			},
		},
		{
			name:    "Flattened page 114660: commercial alarm",
			rawJSON: `{"raw_text":"POCSAG512: Address:    1024  Function: 2  Alpha:   ALARM-COMMERCIAL/HR/MULTI RES/CHURCH/HOSP/SCHOOL  15:27:32 A-10 YORK ST  Dv: 1  Sec: 13E  Gd: BE53 SUSSEX DR & BY WARD MARKET SQ P13A P57 T13 DC50 #114660 Sent by: FDP1"}`,
			expected: models.Message{
				Type: "CALL", CallType: "ALARM-COMMERCIAL/HR/MULTI RES/CHURCH/HOSP/SCHOOL", IncidentNumber: "26-114660",
				Vehicles: []string{"P13A", "P57", "T13", "DC50"},
				Address:  "A-10 YORK ST", Intersection: "SUSSEX DR & BY WARD MARKET SQ",
				Source: "1024", Stations: []string{"13", "57", "50"}, DV: "1", Section: "13E", Grid: "BE53",
			},
		},
		{
			name:    "Flattened page 114639: commercial alarm",
			rawJSON: `{"raw_text":"POCSAG512: Address:    1024  Function: 2  Alpha:   ALARM-COMMERCIAL/HR/MULTI RES/CHURCH/HOSP/SCHOOL  13:47:03 400 INLET PRIV  Dv: 5  Sec: 53E  Gd: BM69 JEANNE DARC BLVD N P53 P55 LA53 DC50 #114639 Sent by: FDP1<NUL>"}`,
			expected: models.Message{
				Type: "CALL", CallType: "ALARM-COMMERCIAL/HR/MULTI RES/CHURCH/HOSP/SCHOOL", IncidentNumber: "26-114639",
				Vehicles: []string{"P53", "P55", "LA53", "DC50"},
				Address:  "400 INLET PRIV", Intersection: "JEANNE DARC BLVD N",
				Source: "1024", Stations: []string{"53", "55", "50"}, DV: "5", Section: "53E", Grid: "BM69",
			},
		},
		{
			name:    "Flattened outside-fire page recognizes UTV unit",
			rawJSON: `{"raw_text":"POCSAG512: Address:  930032  Function: 2  Alpha:   OUTSIDE FIRE-NO EXP.  11:17:08 3057 GREENLAND RD  Dv: 6  Sec: 66E  Gd: BG28 VANCES SIDE RD & SCOTCH PINE GRV UTV63 #116838 Sent by: FDP1<NUL>"}`,
			expected: models.Message{
				Type: "CALL", CallType: "OUTSIDE FIRE-NO EXP.", IncidentNumber: "26-116838",
				Vehicles: []string{"UTV63"},
				Address:  "3057 GREENLAND RD", Intersection: "VANCES SIDE RD & SCOTCH PINE GRV",
				Source: "930032", Stations: []string{"63"}, DV: "6", Section: "66E", Grid: "BG28",
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
			if msg.Intersection != s.expected.Intersection {
				t.Errorf("Expected Intersection %s, got %s", s.expected.Intersection, msg.Intersection)
			}
			if msg.DV != s.expected.DV || msg.Section != s.expected.Section || msg.Grid != s.expected.Grid || msg.Alias != s.expected.Alias {
				t.Errorf("Expected dispatch fields dv=%q sec=%q gd=%q alias=%q, got dv=%q sec=%q gd=%q alias=%q",
					s.expected.DV, s.expected.Section, s.expected.Grid, s.expected.Alias,
					msg.DV, msg.Section, msg.Grid, msg.Alias)
			}
			if msg.Source != s.expected.Source {
				t.Errorf("Expected source %s, got %s", s.expected.Source, msg.Source)
			}
			if !reflect.DeepEqual(msg.Stations, s.expected.Stations) {
				t.Errorf("Expected stations %v, got %v", s.expected.Stations, msg.Stations)
			}
		})
	}
}

func TestPagerFrameFilter(t *testing.T) {
	valid := []string{
		"POCSAG512: Address:  930032  Function: 2  Alpha:   SMOKE VISIBLE<EOT><NUL>",
		"POCSAG1200: Address: 123456 Function: 0 Alpha:",
		"POCSAG2400: Address: 123456",
	}
	for _, raw := range valid {
		if !isPagerFrame(raw) {
			t.Errorf("expected valid pager frame to pass: %q", raw)
		}
	}
	startup := []string{
		"multimon-ng 1.3.1",
		"Available demodulators: POCSAG512 POCSAG1200 POCSAG2400 FLEX FLEX_NEXT EAS",
		"Found Rafael Micro R820T tuner",
		"Tuner gain set to 44.50 dB.",
		"[R82XX] PLL not locked!",
		"",
	}
	for _, raw := range startup {
		if isPagerFrame(raw) {
			t.Errorf("unexpectedly accepted SDR diagnostic as a pager frame: %q", raw)
		}
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
