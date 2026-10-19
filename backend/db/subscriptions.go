package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/firepager/backend/models"
)

func GetSubscriptionSettings(userID int) (*models.SubscriptionSettings, error) {
	settings := &models.SubscriptionSettings{Enabled: true, Rules: []models.SubscriptionRule{}, Channels: []models.NotificationChannel{}}
	if err := DB.QueryRow(`SELECT enabled FROM user_subscription_settings WHERE user_id = $1`, userID).Scan(&settings.Enabled); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	rows, err := DB.Query(`SELECT id, station, call_type, enabled FROM user_subscription_rules WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rule models.SubscriptionRule
		if err := rows.Scan(&rule.ID, &rule.Station, &rule.CallType, &rule.Enabled); err != nil {
			return nil, err
		}
		settings.Rules = append(settings.Rules, rule)
	}
	rows, err = DB.Query(`SELECT id, type, address, enabled FROM user_notification_channels WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var channel models.NotificationChannel
		if err := rows.Scan(&channel.ID, &channel.Type, &channel.Address, &channel.Enabled); err != nil {
			return nil, err
		}
		settings.Channels = append(settings.Channels, channel)
	}
	return settings, rows.Err()
}

func SaveSubscriptionSettings(userID int, settings *models.SubscriptionSettings) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO user_subscription_settings (user_id, enabled) VALUES ($1, $2) ON CONFLICT (user_id) DO UPDATE SET enabled = EXCLUDED.enabled`, userID, settings.Enabled); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM user_subscription_rules WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, rule := range settings.Rules {
		if _, err = tx.Exec(`INSERT INTO user_subscription_rules (user_id, station, call_type, enabled) VALUES ($1, $2, $3, $4)`, userID, rule.Station, rule.CallType, rule.Enabled); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`DELETE FROM user_notification_channels WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, channel := range settings.Channels {
		if _, err = tx.Exec(`INSERT INTO user_notification_channels (user_id, type, address, enabled) VALUES ($1, $2, $3, $4)`, userID, channel.Type, channel.Address, channel.Enabled); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func GetSubscriptionCatalogs() (map[string][]models.CatalogEntry, error) {
	result := map[string][]models.CatalogEntry{"stations": {}, "call_types": {}}
	rows, err := DB.Query(`SELECT code, label FROM (
		SELECT code, label FROM subscription_catalog_stations WHERE is_active
		UNION SELECT DISTINCT BTRIM(s), BTRIM(s) FROM messages, regexp_split_to_table(station, ',') AS s WHERE type = 'CALL' AND BTRIM(s) <> ''
	) catalog ORDER BY code`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e models.CatalogEntry
		if err := rows.Scan(&e.Code, &e.Label); err != nil {
			rows.Close()
			return nil, err
		}
		result["stations"] = append(result["stations"], e)
	}
	rows.Close()
	rows, err = DB.Query(`SELECT code, label FROM (
		SELECT code, label FROM subscription_catalog_call_types WHERE is_active
		UNION SELECT DISTINCT call_type, call_type FROM messages WHERE call_type IS NOT NULL AND BTRIM(call_type) <> ''
	) catalog ORDER BY code`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e models.CatalogEntry
		if err := rows.Scan(&e.Code, &e.Label); err != nil {
			rows.Close()
			return nil, err
		}
		result["call_types"] = append(result["call_types"], e)
	}
	rows.Close()
	return result, nil
}

// Keep push as a legacy alias while accepting the channel identifiers exposed
// by the provider catalog. Saving a subscription does not require a provider
// to be usable yet; that state is reported by the catalog.
var validChannelTypes = map[string]bool{
	"sms": true, "email": true, "push": true, "whatsapp": true,
	"telegram": true, "signal": true, "pushover": true, "ntfy": true,
	"web_push": true, "discord": true, "slack": true, "teams": true, "voice": true,
}

func NormalizeAndValidateSubscriptionSettings(settings *models.SubscriptionSettings) error {
	for i := range settings.Rules {
		settings.Rules[i].Station = strings.TrimSpace(settings.Rules[i].Station)
		settings.Rules[i].CallType = strings.TrimSpace(settings.Rules[i].CallType)
		if settings.Rules[i].Station == "" && settings.Rules[i].CallType == "" {
			return fmt.Errorf("rule %d must specify station or call_type", i)
		}
	}
	for i := range settings.Channels {
		settings.Channels[i].Type = strings.ToLower(strings.TrimSpace(settings.Channels[i].Type))
		settings.Channels[i].Address = strings.TrimSpace(settings.Channels[i].Address)
		if !validChannelTypes[settings.Channels[i].Type] {
			return fmt.Errorf("channel %d has unsupported type", i)
		}
		if settings.Channels[i].Address == "" {
			return fmt.Errorf("channel %d address is required", i)
		}
	}
	return nil
}
