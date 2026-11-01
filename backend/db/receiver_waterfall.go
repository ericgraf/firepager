package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/firepager/backend/models"
)

// DefaultWaterfallRetention defines the maximum age of retained waterfall snapshots (48 hours).
const DefaultWaterfallRetention = 48 * time.Hour

var (
	knownReceiverMu  sync.RWMutex
	knownReceiverIDs = map[string]bool{
		"remote-pi-sdr": true,
		"host-sdr-0":    true,
		"host-sdr-1":    true,
		"host-sdr-2":    true,
	}
)

// SetKnownReceiverForTest registers a known receiver ID in the in-memory fallback map (useful for tests).
func SetKnownReceiverForTest(id string, known bool) {
	knownReceiverMu.Lock()
	defer knownReceiverMu.Unlock()
	if known {
		knownReceiverIDs[id] = true
	} else {
		delete(knownReceiverIDs, id)
	}
}

// IsKnownReceiver reports whether receiverID is a recognized receiver in the fleet.
// It checks the static fleet set as well as any live receiver registered in receiver_status.
func IsKnownReceiver(receiverID string) bool {
	receiverID = strings.TrimSpace(receiverID)
	if receiverID == "" || len(receiverID) > 100 {
		return false
	}

	knownReceiverMu.RLock()
	known := knownReceiverIDs[receiverID]
	knownReceiverMu.RUnlock()
	if known {
		return true
	}

	if DB != nil {
		var exists bool
		err := DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM receiver_status WHERE receiver_id = $1)`, receiverID).Scan(&exists)
		if err == nil && exists {
			return true
		}
	}

	return false
}

// SaveReceiverWaterfall persists a waterfall snapshot within a database transaction,
// purges expired snapshots beyond retention, and commits atomically.
func SaveReceiverWaterfall(w *models.ReceiverWaterfall) error {
	if w == nil {
		return fmt.Errorf("waterfall payload cannot be nil")
	}
	if w.ReceiverID == "" || w.CapturedAt.IsZero() {
		return fmt.Errorf("receiver_id and captured_at are required")
	}
	if DB == nil {
		return fmt.Errorf("database connection is not initialized")
	}

	rowsJSON, err := json.Marshal(w.Rows)
	if err != nil {
		return fmt.Errorf("marshaling waterfall rows: %w", err)
	}

	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	err = tx.QueryRow(`INSERT INTO receiver_waterfalls (
		receiver_id, captured_at, start_hz, step_hz, spectrum_rows, created_at
	) VALUES ($1, $2, $3, $4, $5, NOW())
	RETURNING id, created_at`,
		w.ReceiverID, w.CapturedAt, w.StartHz, w.StepHz, rowsJSON).Scan(&w.ID, &w.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting receiver waterfall: %w", err)
	}

	// Purge records older than retention
	cutoff := time.Now().UTC().Add(-DefaultWaterfallRetention)
	if _, err := tx.Exec(`DELETE FROM receiver_waterfalls WHERE captured_at < $1`, cutoff); err != nil {
		return fmt.Errorf("pruning expired waterfalls: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing waterfall transaction: %w", err)
	}

	return nil
}

// GetReceiverWaterfalls returns the most recent waterfalls for a receiver within the retention window,
// up to the specified limit (defaults to 12 if <= 0).
func GetReceiverWaterfalls(receiverID string, limit int) ([]models.ReceiverWaterfall, error) {
	if receiverID == "" {
		return nil, fmt.Errorf("receiver_id is required")
	}
	if limit <= 0 {
		limit = 12
	}
	if limit > 100 {
		limit = 100
	}

	out := make([]models.ReceiverWaterfall, 0)
	if DB == nil {
		return out, nil
	}

	cutoff := time.Now().UTC().Add(-DefaultWaterfallRetention)
	rows, err := DB.Query(`SELECT id, receiver_id, captured_at, start_hz, step_hz, spectrum_rows, created_at
		FROM receiver_waterfalls
		WHERE receiver_id = $1 AND captured_at >= $2
		ORDER BY captured_at DESC
		LIMIT $3`, receiverID, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("querying receiver waterfalls: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item models.ReceiverWaterfall
		var rowsJSON []byte
		if err := rows.Scan(&item.ID, &item.ReceiverID, &item.CapturedAt, &item.StartHz, &item.StepHz, &rowsJSON, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning receiver waterfall: %w", err)
		}
		if len(rowsJSON) > 0 {
			if err := json.Unmarshal(rowsJSON, &item.Rows); err != nil {
				return nil, fmt.Errorf("unmarshaling waterfall rows: %w", err)
			}
		}
		if item.Rows == nil {
			item.Rows = make([]models.WaterfallRow, 0)
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

// PruneReceiverWaterfalls deletes records older than the given retention duration.
func PruneReceiverWaterfalls(retention time.Duration) (int64, error) {
	if DB == nil {
		return 0, fmt.Errorf("database connection is not initialized")
	}
	cutoff := time.Now().UTC().Add(-retention)
	res, err := DB.Exec(`DELETE FROM receiver_waterfalls WHERE captured_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
