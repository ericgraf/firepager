package db

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/firepager/backend/models"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var DB *sql.DB

func normalizeStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}

func normalizeUserArrays(u *models.User) {
	u.Capcodes = normalizeStringSlice(u.Capcodes)
	u.Vehicles = normalizeStringSlice(u.Vehicles)
	u.Stations = normalizeStringSlice(u.Stations)
	u.CallTypes = normalizeStringSlice(u.CallTypes)
	u.NotificationTypes = normalizeStringSlice(u.NotificationTypes)
}

func InitDB() {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://backend_user@firepager-db:5432/firepager?sslmode=disable"
	}
	var err error
	DB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	err = DB.Ping()
	if err != nil {
		log.Printf("Cannot ping DB, will retry later: %v", err)
	}

	createSchema()
	seedAdminUser()
}

func createSchema() {
	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id UUID PRIMARY KEY,
		timestamp TIMESTAMP NOT NULL,
		source VARCHAR(50) NOT NULL,
		text TEXT NOT NULL,
		raw_text TEXT,
		type VARCHAR(20),
		page_id VARCHAR(50),
		call_type VARCHAR(100),
		incident_number VARCHAR(50),
		vehicles TEXT[] DEFAULT '{}',
		address TEXT,
		intersection TEXT,
		latitude DOUBLE PRECISION,
		longitude DOUBLE PRECISION,
		client_id VARCHAR(100),
		station VARCHAR(50)
	);
	CREATE TABLE IF NOT EXISTS receiver_status (
		receiver_id VARCHAR(100) PRIMARY KEY,
		host VARCHAR(255) NOT NULL DEFAULT '',
		device_identifier VARCHAR(255) NOT NULL DEFAULT '',
		channel_name VARCHAR(255) NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		nominal_frequency_hz DOUBLE PRECISION,
		configured_gain_db DOUBLE PRECISION,
		configured_ppm INTEGER,
		status VARCHAR(40) NOT NULL DEFAULT 'unknown',
		role VARCHAR(40) NOT NULL DEFAULT 'unknown',
		frequency_hz DOUBLE PRECISION,
		gain_db DOUBLE PRECISION,
		ppm INTEGER,
		active_detector BOOLEAN NOT NULL DEFAULT FALSE,
		decoded_messages_count BIGINT NOT NULL DEFAULT 0,
		calibrations_accepted_count BIGINT NOT NULL DEFAULT 0,
		last_calibration_utc TIMESTAMPTZ,
		observed_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	ALTER TABLE receiver_status ADD COLUMN IF NOT EXISTS device_identifier VARCHAR(255) NOT NULL DEFAULT '';
	ALTER TABLE receiver_status ADD COLUMN IF NOT EXISTS channel_name VARCHAR(255) NOT NULL DEFAULT '';
	ALTER TABLE receiver_status ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
	ALTER TABLE receiver_status ADD COLUMN IF NOT EXISTS nominal_frequency_hz DOUBLE PRECISION;
	ALTER TABLE receiver_status ADD COLUMN IF NOT EXISTS configured_gain_db DOUBLE PRECISION;
	ALTER TABLE receiver_status ADD COLUMN IF NOT EXISTS configured_ppm INTEGER;
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		name VARCHAR(100) NOT NULL,
		phone_number VARCHAR(20) NOT NULL,
		phone_verification_status VARCHAR(20) NOT NULL DEFAULT 'unverified',
		phone_verified_at TIMESTAMP NULL,
		whatsapp_consented BOOLEAN NOT NULL DEFAULT FALSE,
		whatsapp_consent_at TIMESTAMP NULL,
		whatsapp_consent_policy_version VARCHAR(50) NULL,
		capcodes TEXT[] NOT NULL,
		vehicles TEXT[] DEFAULT '{}',
		stations TEXT[] DEFAULT '{}',
		call_types TEXT[] DEFAULT '{}',
		role VARCHAR(20) DEFAULT 'user',
		notification_types TEXT[] DEFAULT '{}',
		pushover_user_key VARCHAR(100) DEFAULT '',
		pushover_app_token VARCHAR(100) DEFAULT '',
		invited_by INTEGER REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS invites (
		id SERIAL PRIMARY KEY,
		code VARCHAR(100) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		created_by INTEGER REFERENCES users(id),
		expires_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP + INTERVAL '7 days'),
		is_used BOOLEAN DEFAULT FALSE,
		used_by INTEGER REFERENCES users(id),
		revoked_at TIMESTAMP NULL
	);

	-- Add columns to users if they don't exist (for existing DBs)
	DO $$ 
	BEGIN 
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='email') THEN
			ALTER TABLE users ADD COLUMN email VARCHAR(255) UNIQUE;
			ALTER TABLE users ADD COLUMN password_hash VARCHAR(255);
			ALTER TABLE users ADD COLUMN role VARCHAR(20) DEFAULT 'user';
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='vehicles') THEN
			ALTER TABLE users ADD COLUMN vehicles TEXT[] DEFAULT '{}';
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='stations') THEN
			ALTER TABLE users ADD COLUMN stations TEXT[] DEFAULT '{}';
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='call_types') THEN
			ALTER TABLE users ADD COLUMN call_types TEXT[] DEFAULT '{}';
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='notification_types') THEN
			ALTER TABLE users ADD COLUMN notification_types TEXT[] DEFAULT '{}';
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='pushover_user_key') THEN
			ALTER TABLE users ADD COLUMN pushover_user_key VARCHAR(100) DEFAULT '';
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='pushover_app_token') THEN
			ALTER TABLE users ADD COLUMN pushover_app_token VARCHAR(100) DEFAULT '';
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='invited_by') THEN
			ALTER TABLE users ADD COLUMN invited_by INTEGER REFERENCES users(id);
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='phone_verification_status') THEN
			ALTER TABLE users ADD COLUMN phone_verification_status VARCHAR(20) NOT NULL DEFAULT 'unverified';
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='phone_verified_at') THEN
			ALTER TABLE users ADD COLUMN phone_verified_at TIMESTAMP NULL;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='whatsapp_consented') THEN
			ALTER TABLE users ADD COLUMN whatsapp_consented BOOLEAN NOT NULL DEFAULT FALSE;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='whatsapp_consent_at') THEN
			ALTER TABLE users ADD COLUMN whatsapp_consent_at TIMESTAMP NULL;
		END IF;
		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='whatsapp_consent_policy_version') THEN
			ALTER TABLE users ADD COLUMN whatsapp_consent_policy_version VARCHAR(50) NULL;
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='invites' AND column_name='expires_at') THEN
			ALTER TABLE invites ADD COLUMN expires_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP + INTERVAL '7 days');
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='invites' AND column_name='revoked_at') THEN
			ALTER TABLE invites ADD COLUMN revoked_at TIMESTAMP NULL;
		END IF;

		IF EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_name='messages' AND column_name='vehicles' AND udt_name <> '_text'
		) THEN
			ALTER TABLE messages ADD COLUMN vehicles_new TEXT[] DEFAULT '{}';
			UPDATE messages
			SET vehicles_new = CASE
				WHEN vehicles IS NULL OR BTRIM(vehicles) = '' THEN ARRAY[]::TEXT[]
				ELSE regexp_split_to_array(BTRIM(vehicles), E'\\s+')
			END;
			ALTER TABLE messages DROP COLUMN vehicles;
			ALTER TABLE messages RENAME COLUMN vehicles_new TO vehicles;
		END IF;

		IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='messages' AND column_name='raw_text') THEN
			ALTER TABLE messages ADD COLUMN raw_text TEXT;
		END IF;
	END $$;

	CREATE TABLE IF NOT EXISTS system_configs (
		key VARCHAR(100) PRIMARY KEY,
		value TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS subscription_catalog_stations (
		code VARCHAR(50) PRIMARY KEY,
		label VARCHAR(100) NOT NULL,
		is_active BOOLEAN NOT NULL DEFAULT TRUE
	);
	CREATE TABLE IF NOT EXISTS subscription_catalog_call_types (
		code VARCHAR(100) PRIMARY KEY,
		label VARCHAR(150) NOT NULL,
		is_active BOOLEAN NOT NULL DEFAULT TRUE
	);
	CREATE TABLE IF NOT EXISTS user_subscription_settings (
		user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		enabled BOOLEAN NOT NULL DEFAULT TRUE
	);
	CREATE TABLE IF NOT EXISTS user_subscription_rules (
		id SERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		station VARCHAR(50) NOT NULL DEFAULT '',
		call_type VARCHAR(100) NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		UNIQUE (user_id, station, call_type)
	);
	CREATE TABLE IF NOT EXISTS user_notification_channels (
		id SERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		type VARCHAR(20) NOT NULL,
		address VARCHAR(255) NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		UNIQUE (user_id, type, address)
	);
	CREATE TABLE IF NOT EXISTS notification_deliveries (
		id BIGSERIAL PRIMARY KEY,
		message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		-- Deliberately not an FK: subscription edits replace channel rows, while
		-- accepted outbox items retain their channel snapshot.
		channel_id INTEGER NOT NULL,
		channel_type VARCHAR(20) NOT NULL,
		address VARCHAR(255) NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'pending',
		attempts INTEGER NOT NULL DEFAULT 0,
		available_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		lease_until TIMESTAMP NULL,
		last_error TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (message_id, channel_id)
	);
	CREATE INDEX IF NOT EXISTS notification_deliveries_claim_idx ON notification_deliveries (status, available_at, lease_until);
	CREATE TABLE IF NOT EXISTS phone_verification_challenges (
		id BIGSERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		phone_number VARCHAR(20) NOT NULL,
		code_hash VARCHAR(255) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL,
		attempts INTEGER NOT NULL DEFAULT 0,
		consumed_at TIMESTAMP NULL
	);
	CREATE INDEX IF NOT EXISTS phone_verification_user_created_idx ON phone_verification_challenges (user_id, created_at DESC);
	CREATE TABLE IF NOT EXISTS whatsapp_consent_audit (
		id BIGSERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		phone_number VARCHAR(20) NOT NULL,
		consented BOOLEAN NOT NULL,
		policy_version VARCHAR(50) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	-- Query indexes for the high-volume message list and time-window metrics.
	CREATE INDEX IF NOT EXISTS messages_timestamp_id_idx ON messages (timestamp DESC, id DESC);
	CREATE INDEX IF NOT EXISTS messages_type_timestamp_idx ON messages (type, timestamp DESC);
	CREATE INDEX IF NOT EXISTS messages_client_timestamp_idx ON messages (client_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS messages_call_type_timestamp_idx ON messages (call_type, timestamp DESC);
	CREATE INDEX IF NOT EXISTS messages_station_timestamp_idx ON messages (station, timestamp DESC);
	`
	_, err := DB.Exec(schema)
	if err != nil {
		log.Fatalf("Error creating schema: %v", err)
	}
	// Existing message dimensions become available to subscription clients on
	// upgrade. New dimensions are also exposed by GetSubscriptionCatalogs.
	_, _ = DB.Exec(`INSERT INTO subscription_catalog_call_types (code, label)
		SELECT DISTINCT call_type, call_type FROM messages
		WHERE call_type IS NOT NULL AND BTRIM(call_type) <> ''
		ON CONFLICT (code) DO NOTHING`)
	_, _ = DB.Exec(`INSERT INTO subscription_catalog_stations (code, label)
		SELECT DISTINCT BTRIM(s), BTRIM(s) FROM messages, regexp_split_to_table(station, ',') AS s
		WHERE type = 'CALL' AND BTRIM(s) <> ''
		ON CONFLICT (code) DO NOTHING`)

	// Default Configs
	_, _ = DB.Exec("INSERT INTO system_configs (key, value) VALUES ('SMS_GATEWAY_ENABLED', 'false') ON CONFLICT (key) DO NOTHING")
	_, _ = DB.Exec("INSERT INTO system_configs (key, value) VALUES ('WHATSAPP_GATEWAY_ENABLED', 'false') ON CONFLICT (key) DO NOTHING")
	_, _ = DB.Exec("INSERT INTO system_configs (key, value) VALUES ('SMS_PROVIDER', 'twilio') ON CONFLICT (key) DO NOTHING")
	_, _ = DB.Exec("INSERT INTO system_configs (key, value) VALUES ('TWILIO_ACCOUNT_SID', '') ON CONFLICT (key) DO NOTHING")
	_, _ = DB.Exec("INSERT INTO system_configs (key, value) VALUES ('TWILIO_AUTH_TOKEN', '') ON CONFLICT (key) DO NOTHING")
	_, _ = DB.Exec("INSERT INTO system_configs (key, value) VALUES ('TWILIO_FROM_NUMBER', '') ON CONFLICT (key) DO NOTHING")
	_, _ = DB.Exec("INSERT INTO system_configs (key, value) VALUES ('TWILIO_WHATSAPP_FROM_NUMBER', '') ON CONFLICT (key) DO NOTHING")
	_, _ = DB.Exec("INSERT INTO system_configs (key, value) VALUES ('GEOCODING_PROVIDER', 'nominatim') ON CONFLICT (key) DO NOTHING")
	_, _ = DB.Exec("INSERT INTO system_configs (key, value) VALUES ('CITY_REGION_CONTEXT', 'Ottawa, ON') ON CONFLICT (key) DO NOTHING")
}

func seedAdminUser() {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		log.Printf("Error checking users count: %v", err)
		return
	}

	if count == 0 {
		password := os.Getenv("ADMIN_PASSWORD")
		if password == "" {
			log.Println("Skipping admin bootstrap: ADMIN_PASSWORD is not set")
			return
		}
		log.Println("Seeding default admin user...")
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Error hashing default password: %v", err)
			return
		}

		_, err = DB.Exec(`INSERT INTO users (email, password_hash, name, phone_number, capcodes, vehicles, role, notification_types, pushover_user_key, pushover_app_token) 
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			"admin@firepager.local", string(hashedPassword), "Admin", "0000000000",
			pq.Array([]string{"1234567"}), pq.Array([]string{"E12"}), "admin",
			pq.Array([]string{"SMS", "Email", "Push"}), "", "")
		if err != nil {
			log.Printf("Error seeding admin user: %v", err)
		}
	}
}

func SaveMessage(msg *models.Message) error {
	query := `INSERT INTO messages (
		id, timestamp, source, text, raw_text, type, page_id, call_type, incident_number, vehicles, address, intersection, latitude, longitude, client_id, station
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(query,
		msg.ID, msg.Timestamp, msg.Source, msg.Text, msg.RawText, msg.Type,
		msg.PageID, msg.CallType, msg.IncidentNumber, pq.Array(msg.Vehicles),
		msg.Address, msg.Intersection, msg.Latitude, msg.Longitude, msg.ClientID, msg.Station)
	if err != nil {
		return err
	}
	if err = enqueueMatchingDeliveries(tx, msg); err != nil {
		return err
	}
	return tx.Commit()
}

func GetMessages() ([]models.Message, error) {
	page, err := GetMessagesPage(MessageQuery{Limit: 100})
	if err != nil {
		return nil, err
	}
	return page.Messages, nil
}

// MessageQuery describes bounded, keyset-paginated message retrieval.
type MessageQuery struct {
	Limit    int
	Cursor   string
	Type     string
	Source   string
	ClientID string
	Station  string
	CallType string
	Search   string
	From     *time.Time
	To       *time.Time
}

const (
	DefaultMessageLimit = 100
	MaxMessageLimit     = 500
)

// GetMessagesPage uses (timestamp,id) keyset pagination. This remains stable
// when rows are inserted while a client is walking a result set.
func GetMessagesPage(q MessageQuery) (*models.MessagePage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultMessageLimit
	}
	if limit > MaxMessageLimit {
		limit = MaxMessageLimit
	}

	conditions := []string{"TRUE"}
	args := make([]interface{}, 0, 10)
	arg := func(value interface{}) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if q.Type != "" {
		conditions = append(conditions, "type = "+arg(q.Type))
	}
	if q.Source != "" {
		conditions = append(conditions, "source = "+arg(q.Source))
	}
	if q.ClientID != "" {
		conditions = append(conditions, "client_id = "+arg(q.ClientID))
	}
	if q.Station != "" {
		conditions = append(conditions, "station ILIKE "+arg("%"+q.Station+"%"))
	}
	if q.CallType != "" {
		conditions = append(conditions, "call_type = "+arg(q.CallType))
	}
	if q.Search != "" {
		term := "%" + q.Search + "%"
		conditions = append(conditions, "(text ILIKE "+arg(term)+" OR raw_text ILIKE "+arg(term)+" OR address ILIKE "+arg(term)+")")
	}
	if q.From != nil {
		conditions = append(conditions, "timestamp >= "+arg(*q.From))
	}
	if q.To != nil {
		conditions = append(conditions, "timestamp < "+arg(*q.To))
	}
	if q.Cursor != "" {
		cursorTime, cursorID, err := decodeMessageCursor(q.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid message cursor: %w", err)
		}
		conditions = append(conditions, "(timestamp, id) < ("+arg(cursorTime)+", "+arg(cursorID)+")")
	}

	args = append(args, limit+1)
	query := `SELECT id, timestamp, source, text, COALESCE(raw_text, ''), type, page_id, call_type,
		incident_number, vehicles, address, intersection, latitude, longitude, client_id, station
		FROM messages WHERE ` + strings.Join(conditions, " AND ") +
		` ORDER BY timestamp DESC, id DESC LIMIT $` + fmt.Sprint(len(args))
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs := make([]models.Message, 0, limit)
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(
			&m.ID, &m.Timestamp, &m.Source, &m.Text, &m.RawText, &m.Type,
			&m.PageID, &m.CallType, &m.IncidentNumber, pq.Array(&m.Vehicles),
			&m.Address, &m.Intersection, &m.Latitude, &m.Longitude, &m.ClientID, &m.Station,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	page := &models.MessagePage{Messages: msgs}
	if len(msgs) > limit {
		page.HasMore = true
		page.Messages = msgs[:limit]
		page.NextCursor = encodeMessageCursor(page.Messages[limit-1])
	}
	return page, nil
}

func encodeMessageCursor(m models.Message) string {
	return base64.RawURLEncoding.EncodeToString([]byte(m.Timestamp.UTC().Format(time.RFC3339Nano) + "|" + m.ID))
}

func decodeMessageCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", fmt.Errorf("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	return t, parts[1], err
}

func DeleteMessage(id string) error {
	result, err := DB.Exec("DELETE FROM messages WHERE id = $1", id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func GetUsers() ([]models.User, error) {
	query := `
		SELECT
			u.id,
			u.email,
			u.name,
			u.phone_number,
			u.phone_verification_status, u.phone_verified_at,
			u.whatsapp_consented, u.whatsapp_consent_at, u.whatsapp_consent_policy_version,
			u.capcodes,
			u.vehicles,
			u.stations,
			u.call_types,
			u.role,
			u.notification_types,
			u.pushover_user_key,
			u.pushover_app_token,
			u.invited_by,
			COALESCE(inviter.name, '')
		FROM users u
		LEFT JOIN users inviter ON inviter.id = u.invited_by
	`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.Email, &u.Name, &u.PhoneNumber, &u.PhoneVerificationStatus, &u.PhoneVerifiedAt,
			&u.WhatsAppConsented, &u.WhatsAppConsentAt, &u.WhatsAppConsentPolicyVersion,
			pq.Array(&u.Capcodes),
			pq.Array(&u.Vehicles),
			pq.Array(&u.Stations),
			pq.Array(&u.CallTypes),
			&u.Role,
			pq.Array(&u.NotificationTypes),
			&u.PushoverUserKey,
			&u.PushoverAppToken,
			&u.InvitedBy,
			&u.InvitedByName,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func AddUser(u *models.User) error {
	normalizeUserArrays(u)
	phone, err := NormalizeE164(u.PhoneNumber)
	if err != nil {
		return err
	}
	u.PhoneNumber = phone

	query := "INSERT INTO users (email, password_hash, name, phone_number, capcodes, vehicles, stations, call_types, role, notification_types, pushover_user_key, pushover_app_token, invited_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id"
	return DB.QueryRow(query,
		u.Email, u.PasswordHash, u.Name, u.PhoneNumber,
		pq.Array(u.Capcodes),
		pq.Array(u.Vehicles),
		pq.Array(u.Stations),
		pq.Array(u.CallTypes),
		u.Role,
		pq.Array(u.NotificationTypes),
		u.PushoverUserKey,
		u.PushoverAppToken,
		u.InvitedBy,
	).Scan(&u.ID)
}

func GetUserByEmail(email string) (*models.User, error) {
	var u models.User
	query := `
		SELECT
			u.id,
			u.email,
			u.password_hash,
			u.name,
			u.phone_number,
			u.phone_verification_status, u.phone_verified_at,
			u.whatsapp_consented, u.whatsapp_consent_at, u.whatsapp_consent_policy_version,
			u.capcodes,
			u.vehicles,
			u.stations,
			u.call_types,
			u.role,
			u.notification_types,
			u.pushover_user_key,
			u.pushover_app_token,
			u.invited_by,
			COALESCE(inviter.name, '')
		FROM users u
		LEFT JOIN users inviter ON inviter.id = u.invited_by
		WHERE u.email = $1
	`
	err := DB.QueryRow(query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.PhoneNumber, &u.PhoneVerificationStatus, &u.PhoneVerifiedAt,
		&u.WhatsAppConsented, &u.WhatsAppConsentAt, &u.WhatsAppConsentPolicyVersion,
		pq.Array(&u.Capcodes),
		pq.Array(&u.Vehicles),
		pq.Array(&u.Stations),
		pq.Array(&u.CallTypes),
		&u.Role,
		pq.Array(&u.NotificationTypes),
		&u.PushoverUserKey,
		&u.PushoverAppToken,
		&u.InvitedBy,
		&u.InvitedByName,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func GetUserByID(id int) (*models.User, error) {
	var u models.User
	query := `
		SELECT
			u.id,
			u.email,
			u.password_hash,
			u.name,
			u.phone_number,
			u.phone_verification_status, u.phone_verified_at,
			u.whatsapp_consented, u.whatsapp_consent_at, u.whatsapp_consent_policy_version,
			u.capcodes,
			u.vehicles,
			u.stations,
			u.call_types,
			u.role,
			u.notification_types,
			u.pushover_user_key,
			u.pushover_app_token,
			u.invited_by,
			COALESCE(inviter.name, '')
		FROM users u
		LEFT JOIN users inviter ON inviter.id = u.invited_by
		WHERE u.id = $1
	`
	err := DB.QueryRow(query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.PhoneNumber, &u.PhoneVerificationStatus, &u.PhoneVerifiedAt,
		&u.WhatsAppConsented, &u.WhatsAppConsentAt, &u.WhatsAppConsentPolicyVersion,
		pq.Array(&u.Capcodes),
		pq.Array(&u.Vehicles),
		pq.Array(&u.Stations),
		pq.Array(&u.CallTypes),
		&u.Role,
		pq.Array(&u.NotificationTypes),
		&u.PushoverUserKey,
		&u.PushoverAppToken,
		&u.InvitedBy,
		&u.InvitedByName,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func UpdateUser(u *models.User) error {
	normalizeUserArrays(u)
	phone, err := NormalizeE164(u.PhoneNumber)
	if err != nil {
		return err
	}
	var oldPhone string
	if err := DB.QueryRow(`SELECT phone_number FROM users WHERE id=$1`, u.ID).Scan(&oldPhone); err != nil {
		return err
	}
	phoneChanged := phone != oldPhone
	u.PhoneNumber = phone

	query := `UPDATE users SET 
		email = $1, name = $2, phone_number = $3, 
		capcodes = $4, vehicles = $5, stations = $6, 
		call_types = $7, notification_types = $8,
		pushover_user_key = $9, pushover_app_token = $10`
	if phoneChanged {
		query += ", phone_verification_status = 'unverified', phone_verified_at = NULL, whatsapp_consented = FALSE, whatsapp_consent_at = NULL, whatsapp_consent_policy_version = NULL"
	}

	params := []interface{}{
		u.Email, u.Name, u.PhoneNumber,
		pq.Array(u.Capcodes), pq.Array(u.Vehicles), pq.Array(u.Stations),
		pq.Array(u.CallTypes), pq.Array(u.NotificationTypes),
		u.PushoverUserKey, u.PushoverAppToken,
	}

	if u.PasswordHash != "" {
		query += ", password_hash = $11 WHERE id = $12"
		params = append(params, u.PasswordHash, u.ID)
	} else {
		query += " WHERE id = $11"
		params = append(params, u.ID)
	}

	_, err = DB.Exec(query, params...)
	return err
}

func UpdateUserRole(userID int, role string) error {
	_, err := DB.Exec("UPDATE users SET role = $1 WHERE id = $2", role, userID)
	return err
}

func GetConfigs() ([]models.SystemConfig, error) {
	rows, err := DB.Query("SELECT key, value FROM system_configs")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []models.SystemConfig
	for rows.Next() {
		var c models.SystemConfig
		if err := rows.Scan(&c.Key, &c.Value); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, nil
}

func UpdateConfig(key, value string) error {
	_, err := DB.Exec("INSERT INTO system_configs (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2", key, value)
	return err
}

func GetConfigValue(key, fallback string) string {
	var value string
	err := DB.QueryRow("SELECT value FROM system_configs WHERE key = $1", key).Scan(&value)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}

func UpdateMessageCoordinates(messageID string, latitude, longitude float64) error {
	_, err := DB.Exec(
		"UPDATE messages SET latitude = $1, longitude = $2 WHERE id = $3",
		latitude,
		longitude,
		messageID,
	)
	return err
}

func CreateInvite(invite *models.Invite) error {
	query := "INSERT INTO invites (code, created_by, expires_at) VALUES ($1, $2, $3) RETURNING id, created_at, expires_at"
	return DB.QueryRow(query, invite.Code, invite.CreatedBy, invite.ExpiresAt).Scan(&invite.ID, &invite.CreatedAt, &invite.ExpiresAt)
}

func GetInviteByCode(code string) (*models.Invite, error) {
	var inv models.Invite
	query := `
		SELECT
			i.id,
			i.code,
			i.created_at,
			i.created_by,
			COALESCE(creator.name, ''),
			i.expires_at,
			i.is_used,
			i.used_by,
			COALESCE(used_user.name, ''),
			i.revoked_at
		FROM invites i
		LEFT JOIN users creator ON creator.id = i.created_by
		LEFT JOIN users used_user ON used_user.id = i.used_by
		WHERE i.code = $1
	`
	err := DB.QueryRow(query, code).Scan(
		&inv.ID,
		&inv.Code,
		&inv.CreatedAt,
		&inv.CreatedBy,
		&inv.CreatedByName,
		&inv.ExpiresAt,
		&inv.IsUsed,
		&inv.UsedBy,
		&inv.UsedByName,
		&inv.RevokedAt,
	)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func MarkInviteUsed(code string, userID int) error {
	query := "UPDATE invites SET is_used = TRUE, used_by = $1 WHERE code = $2 AND NOT is_used AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP"
	_, err := DB.Exec(query, userID, code)
	return err
}

func GetInvites() ([]models.Invite, error) {
	query := `
		SELECT
			i.id,
			i.code,
			i.created_at,
			i.created_by,
			COALESCE(creator.name, ''),
			i.expires_at,
			i.is_used,
			i.used_by,
			COALESCE(used_user.name, ''),
			i.revoked_at
		FROM invites i
		LEFT JOIN users creator ON creator.id = i.created_by
		LEFT JOIN users used_user ON used_user.id = i.used_by
		ORDER BY i.created_at DESC
	`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []models.Invite
	for rows.Next() {
		var inv models.Invite
		if err := rows.Scan(&inv.ID, &inv.Code, &inv.CreatedAt, &inv.CreatedBy, &inv.CreatedByName, &inv.ExpiresAt, &inv.IsUsed, &inv.UsedBy, &inv.UsedByName, &inv.RevokedAt); err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	return invites, nil
}

// GetInvitesForUser returns only invites created by the authenticated user.
func GetInvitesForUser(userID int) ([]models.Invite, error) {
	// Keep the query and scan shape identical to GetInvites while restricting ownership.
	query := `SELECT i.id, i.code, i.created_at, i.created_by, COALESCE(creator.name, ''), i.expires_at,
		i.is_used, i.used_by, COALESCE(used_user.name, ''), i.revoked_at
		FROM invites i LEFT JOIN users creator ON creator.id = i.created_by LEFT JOIN users used_user ON used_user.id = i.used_by
		WHERE i.created_by = $1 ORDER BY i.created_at DESC`
	rows, err := DB.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invites []models.Invite
	for rows.Next() {
		var inv models.Invite
		if err := rows.Scan(&inv.ID, &inv.Code, &inv.CreatedAt, &inv.CreatedBy, &inv.CreatedByName, &inv.ExpiresAt, &inv.IsUsed, &inv.UsedBy, &inv.UsedByName, &inv.RevokedAt); err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	return invites, rows.Err()
}

func RevokeInvite(id, userID int, admin bool) error {
	query := "UPDATE invites SET revoked_at = CURRENT_TIMESTAMP WHERE id = $1 AND revoked_at IS NULL AND NOT is_used"
	args := []interface{}{id}
	if !admin {
		query += " AND created_by = $2"
		args = append(args, userID)
	}
	result, err := DB.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AddUserWithInvite atomically locks and consumes an invite before inserting the user.
func AddUserWithInvite(u *models.User, code string) error {
	normalizeUserArrays(u)
	phone, err := NormalizeE164(u.PhoneNumber)
	if err != nil {
		return err
	}
	u.PhoneNumber = phone
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var inviter int
	err = tx.QueryRow(`SELECT created_by FROM invites WHERE code = $1 AND NOT is_used AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP FOR UPDATE`, code).Scan(&inviter)
	if err != nil {
		return err
	}
	u.InvitedBy = &inviter
	err = tx.QueryRow("INSERT INTO users (email, password_hash, name, phone_number, capcodes, vehicles, stations, call_types, role, notification_types, pushover_user_key, pushover_app_token, invited_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id", u.Email, u.PasswordHash, u.Name, u.PhoneNumber, pq.Array(u.Capcodes), pq.Array(u.Vehicles), pq.Array(u.Stations), pq.Array(u.CallTypes), u.Role, pq.Array(u.NotificationTypes), u.PushoverUserKey, u.PushoverAppToken, u.InvitedBy).Scan(&u.ID)
	if err != nil {
		return err
	}
	result, err := tx.Exec("UPDATE invites SET is_used = TRUE, used_by = $1 WHERE code = $2", u.ID, code)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func GetMetrics() (*models.Metrics, error) {
	return GetMetricsFiltered(MetricsQuery{})
}

type MetricsQuery struct {
	Type     string
	ClientID string
	Station  string
	CallType string
	From     *time.Time
	To       *time.Time
}

// GetMetricsFiltered applies the same time and dimension filters to every
// metric. Aggregation stays in PostgreSQL, rather than loading the message
// table into the API process.
func GetMetricsFiltered(q MetricsQuery) (*models.Metrics, error) {
	metrics := &models.Metrics{}
	conditions := []string{"TRUE"}
	args := make([]interface{}, 0, 6)
	add := func(value interface{}) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if q.Type != "" {
		conditions = append(conditions, "type = "+add(q.Type))
	}
	if q.ClientID != "" {
		conditions = append(conditions, "client_id = "+add(q.ClientID))
	}
	if q.Station != "" {
		conditions = append(conditions, "station ILIKE "+add("%"+q.Station+"%"))
	}
	if q.CallType != "" {
		conditions = append(conditions, "call_type = "+add(q.CallType))
	}
	if q.From != nil {
		conditions = append(conditions, "timestamp >= "+add(*q.From))
	}
	if q.To != nil {
		conditions = append(conditions, "timestamp < "+add(*q.To))
	}
	where := strings.Join(conditions, " AND ")

	err := DB.QueryRow("SELECT COUNT(*) FROM messages WHERE "+where, args...).Scan(&metrics.TotalMessages)
	if err != nil {
		return nil, err
	}

	// By Type
	rows, err := DB.Query("SELECT type, COUNT(*) FROM messages WHERE "+where+" GROUP BY type ORDER BY COUNT(*) DESC, type ASC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s models.Stat
		var t sql.NullString
		if err := rows.Scan(&t, &s.Value); err != nil {
			return nil, err
		}
		if t.Valid {
			s.Label = t.String
		} else {
			s.Label = "UNKNOWN"
		}
		metrics.ByType = append(metrics.ByType, s)
	}

	// By Client
	rows, err = DB.Query("SELECT client_id, COUNT(*) FROM messages WHERE "+where+" GROUP BY client_id ORDER BY COUNT(*) DESC, client_id ASC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s models.Stat
		var c sql.NullString
		if err := rows.Scan(&c, &s.Value); err != nil {
			return nil, err
		}
		if c.Valid {
			s.Label = c.String
		} else {
			s.Label = "UNKNOWN"
		}
		metrics.ByClient = append(metrics.ByClient, s)
	}

	// By Station (Calls only)
	stationWhere := where + " AND type = 'CALL' AND station <> ''"
	rows, err = DB.Query("SELECT station FROM messages WHERE "+stationWhere, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stationCounts := make(map[string]int)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		for _, station := range strings.Split(raw, ",") {
			station = strings.TrimSpace(station)
			if station == "" {
				continue
			}
			stationCounts[station]++
		}
	}
	for label, value := range stationCounts {
		metrics.ByStation = append(metrics.ByStation, models.Stat{
			Label: label,
			Value: value,
		})
	}
	sort.Slice(metrics.ByStation, func(i, j int) bool {
		if metrics.ByStation[i].Value == metrics.ByStation[j].Value {
			return metrics.ByStation[i].Label < metrics.ByStation[j].Label
		}
		return metrics.ByStation[i].Value > metrics.ByStation[j].Value
	})

	return metrics, nil
}
