package models

import "time"

type Message struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Source         string    `json:"source"`
	Text           string    `json:"text"`
	RawText        string    `json:"raw_text"`
	Type           string    `json:"type"`
	PageID         string    `json:"page_id"`
	CallType       string    `json:"call_type"`
	IncidentNumber string    `json:"incident_number"`
	Vehicles       []string  `json:"vehicles"`
	Address        string    `json:"address"`
	Intersection   string    `json:"intersection"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
	ClientID       string    `json:"client_id"`
	Station        string    `json:"station"`
}

// MessagePage is returned by the paginated messages endpoint. Cursor is an
// opaque value so clients do not need to know about the database sort key.
type MessagePage struct {
	Messages   []Message `json:"messages"`
	NextCursor string    `json:"next_cursor,omitempty"`
	HasMore    bool      `json:"has_more"`
}

type User struct {
	ID                int      `json:"id"`
	Email             string   `json:"email"`
	PasswordHash      string   `json:"-"`
	Name              string   `json:"name"`
	PhoneNumber       string   `json:"phone_number"`
	Capcodes          []string `json:"capcodes"` // Relevant POCSAG CAPCODEs
	Vehicles          []string `json:"vehicles"`
	Stations          []string `json:"stations"`
	CallTypes         []string `json:"call_types"`
	Role              string   `json:"role"`
	NotificationTypes []string `json:"notification_types"`
	PushoverUserKey   string   `json:"pushover_user_key"`
	PushoverAppToken  string   `json:"pushover_app_token"`
	InvitedBy         *int     `json:"invited_by"`
	InvitedByName     string   `json:"invited_by_name,omitempty"`
}

// SubscriptionRule is a dimension match for an alert. An empty station or
// call type is a wildcard for that dimension.
type SubscriptionRule struct {
	ID       int    `json:"id,omitempty"`
	Station  string `json:"station"`
	CallType string `json:"call_type"`
	Enabled  bool   `json:"enabled"`
}

type NotificationChannel struct {
	ID      int    `json:"id,omitempty"`
	Type    string `json:"type"`
	Address string `json:"address"`
	Enabled bool   `json:"enabled"`
}

type SubscriptionSettings struct {
	Enabled  bool                  `json:"enabled"`
	Rules    []SubscriptionRule    `json:"rules"`
	Channels []NotificationChannel `json:"channels"`
}

type CatalogEntry struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type Invite struct {
	ID            int        `json:"id"`
	Code          string     `json:"code"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	CreatedBy     int        `json:"created_by"`
	CreatedByName string     `json:"created_by_name,omitempty"`
	IsUsed        bool       `json:"is_used"`
	UsedBy        *int       `json:"used_by"`
	UsedByName    string     `json:"used_by_name,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

type SystemConfig struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Stat struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

type Metrics struct {
	TotalMessages int    `json:"total_messages"`
	ByType        []Stat `json:"by_type"`
	ByClient      []Stat `json:"by_client"`
	ByStation     []Stat `json:"by_station"`
}
