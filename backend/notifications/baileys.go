package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// BaileysConfig is deliberately separate from MetaCloudConfig. Baileys is an
// unofficial, local bridge and must never be selected by changing Meta keys.
type BaileysConfig struct {
	Enabled  bool
	Endpoint string
	Token    string
	Timeout  time.Duration
}

func ParseBaileysConfig(values map[string]string) BaileysConfig {
	get := func(key string) string {
		if v := strings.TrimSpace(values[key]); v != "" {
			return v
		}
		return strings.TrimSpace(getenv(key))
	}
	seconds := 10
	if v := get("WHATSAPP_BRIDGE_TIMEOUT_SECONDS"); v != "" {
		if n := atoi(v); n > 0 && n <= 120 {
			seconds = n
		}
	}
	return BaileysConfig{
		Enabled:  configTruthy(get("WHATSAPP_GATEWAY_ENABLED")) && strings.EqualFold(get("WHATSAPP_PROVIDER"), "baileys"),
		Endpoint: strings.TrimRight(get("WHATSAPP_BRIDGE_URL"), "/"), Token: get("WHATSAPP_BRIDGE_AUTH_TOKEN"), Timeout: time.Duration(seconds) * time.Second,
	}
}

// Small wrappers keep configuration parsing testable without exposing secrets.
var getenv = func(key string) string { return os.Getenv(key) }
var atoi = func(value string) int { n, _ := strconv.Atoi(value); return n }

type BaileysSender struct {
	Config BaileysConfig
	Client *http.Client
}

type BaileysPairingStatus struct {
	Connection string `json:"connection"`
	QR         string `json:"qr,omitempty"`
}

// GetBaileysPairingStatus reads the bridge's authenticated pairing status.
// Callers must authorize an administrator before returning a QR because it
// grants access to the shared linked WhatsApp account.
func GetBaileysPairingStatus(ctx context.Context, cfg BaileysConfig, client *http.Client) (BaileysPairingStatus, error) {
	if !cfg.Enabled || cfg.Endpoint == "" || cfg.Token == "" {
		return BaileysPairingStatus{}, errors.New("whatsapp bridge is not configured")
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.Endpoint+"/pairing", nil)
	if err != nil {
		return BaileysPairingStatus{}, errors.New("whatsapp pairing request could not be created")
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	resp, err := client.Do(req)
	if err != nil {
		return BaileysPairingStatus{}, errors.New("whatsapp pairing status is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BaileysPairingStatus{}, errors.New("whatsapp pairing status is unavailable")
	}
	var status BaileysPairingStatus
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&status); err != nil || strings.TrimSpace(status.Connection) == "" {
		return BaileysPairingStatus{}, errors.New("whatsapp pairing status is invalid")
	}
	return status, nil
}

func NewBaileysSender(cfg BaileysConfig, client *http.Client) *BaileysSender {
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	return &BaileysSender{Config: cfg, Client: client}
}

func (s *BaileysSender) Send(ctx context.Context, request MetaCloudTemplateRequest) error {
	if s == nil || !s.Config.Enabled || s.Config.Endpoint == "" || s.Config.Token == "" {
		return errors.New("whatsapp provider is not configured")
	}
	text := ""
	if len(request.Template.Components) > 0 && len(request.Template.Components[0].Parameters) > 0 {
		text = request.Template.Components[0].Parameters[0].Text
	}
	payload, err := json.Marshal(struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}{request.To, text})
	if err != nil {
		return errors.New("whatsapp bridge request could not be encoded")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Config.Endpoint+"/send", strings.NewReader(string(payload)))
	if err != nil {
		return errors.New("whatsapp bridge request could not be created")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Config.Token)
	resp, err := s.Client.Do(req)
	if err != nil {
		return RedactProviderError(0)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RedactProviderError(resp.StatusCode)
	}
	return nil
}
