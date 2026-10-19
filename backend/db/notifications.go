package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/firepager/backend/models"
	"github.com/lib/pq"
)

// enqueueMatchingDeliveries runs in the same transaction as message insert.
// A rule match is evaluated against the normalized subscription tables; one
// matching rule is enough to create each enabled channel once.
func enqueueMatchingDeliveries(tx *sql.Tx, msg *models.Message) error {
	rows, err := tx.Query(`SELECT c.id, c.user_id, c.type, c.address
		FROM user_notification_channels c
		JOIN user_subscription_settings s ON s.user_id = c.user_id AND s.enabled
		WHERE c.enabled AND EXISTS (
			SELECT 1 FROM user_subscription_rules r
			WHERE r.user_id = c.user_id AND r.enabled
			AND (r.station = '' OR EXISTS (SELECT 1 FROM regexp_split_to_table($1, ',') station WHERE BTRIM(station) ILIKE r.station))
			AND (r.call_type = '' OR r.call_type ILIKE $2)
		)`, msg.Station, msg.CallType)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var channelID, userID int
		var typ, address string
		if err := rows.Scan(&channelID, &userID, &typ, &address); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO notification_deliveries
			(message_id,user_id,channel_id,channel_type,address)
			VALUES ($1,$2,$3,$4,$5) ON CONFLICT (message_id,channel_id) DO NOTHING`,
			msg.ID, userID, channelID, typ, address); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ClaimNotificationDelivery atomically leases one ready delivery. Expired
// leases are reclaimable, allowing a crashed worker to make progress.
func ClaimNotificationDelivery(lease time.Duration) (*models.NotificationDelivery, error) {
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	row := DB.QueryRow(`WITH candidate AS (
		SELECT id FROM notification_deliveries
		WHERE (status = 'pending' OR (status = 'processing' AND lease_until < CURRENT_TIMESTAMP))
		AND available_at <= CURRENT_TIMESTAMP
		ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1
	) UPDATE notification_deliveries d SET status='processing', attempts=attempts+1,
		lease_until=CURRENT_TIMESTAMP + $1 * INTERVAL '1 second', updated_at=CURRENT_TIMESTAMP
		FROM candidate WHERE d.id=candidate.id
		RETURNING d.id,d.message_id,d.user_id,d.channel_id,d.channel_type,d.address,d.status,d.attempts,d.available_at,d.lease_until,d.last_error`, lease.Seconds())
	d := &models.NotificationDelivery{}
	if err := row.Scan(&d.ID, &d.MessageID, &d.UserID, &d.ChannelID, &d.ChannelType, &d.Address, &d.Status, &d.Attempts, &d.AvailableAt, &d.LeaseUntil, &d.LastError); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if err := DB.QueryRow(`SELECT id,timestamp,source,text,raw_text,type,page_id,call_type,incident_number,vehicles,address,intersection,latitude,longitude,client_id,station FROM messages WHERE id=$1`, d.MessageID).Scan(
		&d.Message.ID, &d.Message.Timestamp, &d.Message.Source, &d.Message.Text, &d.Message.RawText, &d.Message.Type, &d.Message.PageID, &d.Message.CallType, &d.Message.IncidentNumber, pq.Array(&d.Message.Vehicles), &d.Message.Address, &d.Message.Intersection, &d.Message.Latitude, &d.Message.Longitude, &d.Message.ClientID, &d.Message.Station); err != nil {
		return nil, fmt.Errorf("load notification message: %w", err)
	}
	return d, nil
}

func CompleteNotificationDelivery(id int64) error {
	_, err := DB.Exec(`UPDATE notification_deliveries SET status='succeeded', lease_until=NULL, updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='processing'`, id)
	return err
}

func FailNotificationDelivery(id int64, message string, retryAt time.Time, dead bool) error {
	status := "pending"
	if dead {
		status = "dead"
	}
	_, err := DB.Exec(`UPDATE notification_deliveries SET status=$1,last_error=$2,available_at=$3,lease_until=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$4 AND status='processing'`, status, message, retryAt, id)
	return err
}
