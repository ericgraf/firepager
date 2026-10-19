package notifications

// This file contains the provider-neutral WhatsApp boundary.  It deliberately
// does not contain an HTTP sender: production wiring must provide and review a
// provider implementation explicitly.  In particular, templates are the only
// supported message shape; free-form message text is never sent by this API.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrWhatsAppDisabled = errors.New("whatsapp is disabled")
	ErrWhatsAppConsent  = errors.New("whatsapp consent is required")
	ErrInvalidE164      = errors.New("invalid E.164 phone number")
)

// MetaCloudConfig contains non-secret settings needed to construct a Meta
// MetaCloudConfig contains Cloud API settings. AccessToken is never included
// in a request payload or an error (it is sent only as an Authorization header).
type MetaCloudConfig struct {
	Enabled       bool
	AccessToken   string
	PhoneNumberID string
	APIVersion    string
	TemplateName  string
	LanguageCode  string
	VerifyToken   string
	AppSecret     string
	Endpoint      string
	Timeout       time.Duration
	Attempts      int
}

// ParseMetaCloudConfig parses configuration without enabling delivery unless
// WHATSAPP_GATEWAY_ENABLED is explicitly truthy.
func ParseMetaCloudConfig(values map[string]string) MetaCloudConfig {
	get := func(keys ...string) string {
		for _, key := range keys {
			if value := strings.TrimSpace(values[key]); value != "" {
				return value
			}
			if value := strings.TrimSpace(os.Getenv(key)); value != "" {
				return value
			}
		}
		return ""
	}
	cfg := MetaCloudConfig{
		Enabled:       configTruthy(get("WHATSAPP_GATEWAY_ENABLED", "META_WHATSAPP_ENABLED")),
		AccessToken:   get("META_WHATSAPP_ACCESS_TOKEN", "WHATSAPP_ACCESS_TOKEN"),
		PhoneNumberID: get("META_WHATSAPP_PHONE_NUMBER_ID", "WHATSAPP_PHONE_NUMBER_ID"),
		APIVersion:    get("META_WHATSAPP_API_VERSION", "WHATSAPP_API_VERSION"),
		TemplateName:  get("META_WHATSAPP_TEMPLATE_NAME", "WHATSAPP_TEMPLATE_NAME"),
		LanguageCode:  get("META_WHATSAPP_TEMPLATE_LANGUAGE", "WHATSAPP_TEMPLATE_LANGUAGE"),
		VerifyToken:   get("META_WHATSAPP_VERIFY_TOKEN", "WHATSAPP_VERIFY_TOKEN"),
		AppSecret:     get("META_WHATSAPP_APP_SECRET", "WHATSAPP_APP_SECRET"),
		Endpoint:      get("META_WHATSAPP_API_URL", "WHATSAPP_API_URL"),
	}
	if seconds, err := strconv.Atoi(get("META_WHATSAPP_TIMEOUT_SECONDS", "WHATSAPP_TIMEOUT_SECONDS")); err == nil && seconds > 0 && seconds <= 120 {
		cfg.Timeout = time.Duration(seconds) * time.Second
	}
	if attempts, err := strconv.Atoi(get("META_WHATSAPP_ATTEMPTS", "WHATSAPP_ATTEMPTS")); err == nil && attempts >= 1 && attempts <= 3 {
		cfg.Attempts = attempts
	}
	return cfg
}

func configTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// IsE164 validates the canonical E.164 representation.
func IsE164(number string) bool {
	if len(number) < 9 || len(number) > 16 || number[0] != '+' || number[1] == '0' {
		return false
	}
	for _, r := range number[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// NormalizeE164 accepts common display separators, but never guesses a
// country code or accepts a local number.
func NormalizeE164(number string) (string, error) {
	number = strings.TrimSpace(number)
	if number == "" {
		return "", ErrInvalidE164
	}
	var b strings.Builder
	for i, r := range number {
		if i == 0 && r == '+' {
			b.WriteRune(r)
			continue
		}
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if strings.ContainsRune(" -().", r) {
			continue
		}
		return "", ErrInvalidE164
	}
	result := b.String()
	if !IsE164(result) {
		return "", ErrInvalidE164
	}
	return result, nil
}

// ConsentGate validates both consent and the destination before any provider
// is called. Keeping this separate makes it usable by other notification
// orchestration code without importing a provider.
func ConsentGate(phone string, consent bool) (string, error) {
	if !consent {
		return "", ErrWhatsAppConsent
	}
	return NormalizeE164(phone)
}

type TemplateParameter struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type TemplateComponent struct {
	Type       string              `json:"type"`
	Parameters []TemplateParameter `json:"parameters,omitempty"`
}

type MetaCloudTemplateRequest struct {
	MessagingProduct string         `json:"messaging_product"`
	To               string         `json:"to"`
	Type             string         `json:"type"`
	Template         TemplateObject `json:"template"`
}

type TemplateObject struct {
	Name       string              `json:"name"`
	Language   TemplateLanguage    `json:"language"`
	Components []TemplateComponent `json:"components,omitempty"`
}

type TemplateLanguage struct {
	Code string `json:"code"`
}

// BuildMetaCloudTemplateRequest constructs a template-only Meta payload.
func BuildMetaCloudTemplateRequest(cfg MetaCloudConfig, phone string, variables []string) (MetaCloudTemplateRequest, error) {
	to, err := NormalizeE164(phone)
	if err != nil {
		return MetaCloudTemplateRequest{}, err
	}
	if strings.TrimSpace(cfg.TemplateName) == "" || strings.TrimSpace(cfg.LanguageCode) == "" {
		return MetaCloudTemplateRequest{}, errors.New("whatsapp template configuration is incomplete")
	}
	params := make([]TemplateParameter, len(variables))
	for i, value := range variables {
		params[i] = TemplateParameter{Type: "text", Text: value}
	}
	components := []TemplateComponent(nil)
	if len(params) > 0 {
		components = []TemplateComponent{{Type: "body", Parameters: params}}
	}
	return MetaCloudTemplateRequest{
		MessagingProduct: "whatsapp", To: to, Type: "template",
		Template: TemplateObject{Name: cfg.TemplateName, Language: TemplateLanguage{Code: cfg.LanguageCode}, Components: components},
	}, nil
}

// WhatsAppProvider is intentionally small so Meta, Twilio, or a test double
// can be supplied without changing notification orchestration.
type WhatsAppProvider interface {
	Send(context.Context, MetaCloudTemplateRequest) error
}

// MockWhatsAppProvider records requests and never performs network I/O.
type MockWhatsAppProvider struct {
	Requests []MetaCloudTemplateRequest
}

func NewMockWhatsAppProvider() *MockWhatsAppProvider { return &MockWhatsAppProvider{} }

func (m *MockWhatsAppProvider) Send(_ context.Context, request MetaCloudTemplateRequest) error {
	if m == nil {
		return errors.New("nil whatsapp mock provider")
	}
	m.Requests = append(m.Requests, request)
	return nil
}

// WhatsAppAdapter applies the safety gates before handing a template to a
// provider. It is disabled unless config explicitly enables it.
type WhatsAppAdapter struct {
	Config   MetaCloudConfig
	Provider WhatsAppProvider
}

func (a WhatsAppAdapter) SendTemplate(ctx context.Context, phone string, consent bool, variables []string) error {
	if !a.Config.Enabled {
		return ErrWhatsAppDisabled
	}
	if a.Provider == nil {
		return errors.New("whatsapp provider is not configured")
	}
	to, err := ConsentGate(phone, consent)
	if err != nil {
		return err
	}
	request, err := BuildMetaCloudTemplateRequest(a.Config, to, variables)
	if err != nil {
		return err
	}
	return a.Provider.Send(ctx, request)
}

type RetryClass int

const (
	RetryNo RetryClass = iota
	RetryTransient
	RetryPermanent
)

// ClassifyRetry classifies HTTP/provider failures without inspecting or
// returning response content (which can contain secrets or user message text).
func ClassifyRetry(statusCode int, err error) RetryClass {
	if statusCode == 408 || statusCode == 425 || statusCode == 429 || statusCode >= 500 {
		return RetryTransient
	}
	if statusCode >= 400 {
		return RetryPermanent
	}
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
			return RetryTransient
		}
		return RetryTransient
	}
	return RetryNo
}

// RedactProviderError produces a safe diagnostic containing no response body,
// token, phone number, template variable, or arbitrary provider content.
func RedactProviderError(statusCode int) error {
	if statusCode > 0 {
		return fmt.Errorf("whatsapp provider request failed (status %d)", statusCode)
	}
	return errors.New("whatsapp provider request failed")
}

// RedactSecret is useful for structured logs and tests; it never returns the
// supplied value, including when the value is empty.
func RedactSecret(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<empty>"
	}
	return "<redacted>"
}

// MarshalMetaCloudTemplateRequest is a convenience that makes it harder for
// callers to accidentally marshal a free-form content payload.
func MarshalMetaCloudTemplateRequest(request MetaCloudTemplateRequest) ([]byte, error) {
	return json.Marshal(request)
}

// MetaCloudEndpoint returns the endpoint for a future provider implementation.
func MetaCloudEndpoint(cfg MetaCloudConfig) string {
	version := strings.Trim(cfg.APIVersion, "/")
	if version == "" {
		version = "v20.0"
	}
	return "https://graph.facebook.com/" + version + "/" + strings.TrimSpace(cfg.PhoneNumberID) + "/messages"
}

// RetryAfterSeconds parses a safe numeric Retry-After value. Invalid values
// are ignored rather than echoed into logs.
func RetryAfterSeconds(value string) (int, bool) {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	return seconds, err == nil && seconds >= 0
}

// MetaCloudSender is the concrete Cloud API implementation. It only accepts
// the already validated template request, never free-form message content.
type MetaCloudSender struct {
	Config MetaCloudConfig
	Client *http.Client
}

func NewMetaCloudSender(config MetaCloudConfig, client *http.Client) *MetaCloudSender {
	if config.Timeout <= 0 || config.Timeout > 120*time.Second {
		config.Timeout = 10 * time.Second
	}
	if config.Attempts < 1 || config.Attempts > 3 {
		config.Attempts = 3
	}
	if client == nil {
		client = &http.Client{Timeout: config.Timeout}
	}
	return &MetaCloudSender{Config: config, Client: client}
}

func (s *MetaCloudSender) Send(ctx context.Context, request MetaCloudTemplateRequest) error {
	if s == nil || !s.Config.Enabled || strings.TrimSpace(s.Config.AccessToken) == "" || strings.TrimSpace(s.Config.PhoneNumberID) == "" {
		return errors.New("whatsapp provider is not configured")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return errors.New("whatsapp request could not be encoded")
	}
	endpoint := strings.TrimSpace(s.Config.Endpoint)
	if endpoint == "" {
		endpoint = MetaCloudEndpoint(s.Config)
	}
	for attempt := 0; attempt < s.Config.Attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
		if err != nil {
			return errors.New("whatsapp request could not be created")
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.Config.AccessToken)
		resp, err := s.Client.Do(req)
		if err == nil {
			io.CopyN(io.Discard, resp.Body, 4096)
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			if ClassifyRetry(resp.StatusCode, nil) != RetryTransient || attempt == s.Config.Attempts-1 {
				return RedactProviderError(resp.StatusCode)
			}
		} else if attempt == s.Config.Attempts-1 {
			return RedactProviderError(0)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 50 * time.Millisecond):
		}
	}
	return RedactProviderError(0)
}

// VerifyMetaSignature validates Meta's X-Hub-Signature-256 over the exact
// request bytes. It uses constant-time comparison and never logs the secret.
func VerifyMetaSignature(body []byte, header, appSecret string) bool {
	header = strings.TrimSpace(header)
	if appSecret == "" || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	digest := hmac.New(sha256.New, []byte(appSecret))
	digest.Write(body)
	expected := "sha256=" + hex.EncodeToString(digest.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(header))
}

func VerifyMetaWebhookToken(provided, expected string) bool {
	if provided == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

type webhookEnvelope struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Statuses []MetaStatus `json:"statuses"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type MetaStatus struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// StatusHandler is called once for each status ID. Returning an error causes
// the ID not to be marked handled, allowing a later delivery to retry it.
type StatusHandler func(context.Context, MetaStatus) error

type WebhookProcessor struct {
	Handler StatusHandler
	TTL     time.Duration
	Max     int
	mu      sync.Mutex
	seen    map[string]time.Time
}

func NewWebhookProcessor(handler StatusHandler) *WebhookProcessor {
	return &WebhookProcessor{Handler: handler, TTL: 24 * time.Hour, Max: 4096, seen: make(map[string]time.Time)}
}

func (p *WebhookProcessor) Process(ctx context.Context, body []byte) error {
	var envelope webhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return errors.New("invalid whatsapp webhook")
	}
	for _, entry := range envelope.Entry {
		for _, change := range entry.Changes {
			for _, status := range change.Value.Statuses {
				if strings.TrimSpace(status.ID) == "" {
					continue
				}
				if !p.claim(status.ID) {
					continue
				}
				if p.Handler != nil {
					if err := p.Handler(ctx, status); err != nil {
						p.unclaim(status.ID)
						return err
					}
				}
			}
		}
	}
	return nil
}

func (p *WebhookProcessor) claim(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for key, at := range p.seen {
		if now.Sub(at) > p.TTL {
			delete(p.seen, key)
		}
	}
	if _, ok := p.seen[id]; ok {
		return false
	}
	if p.Max <= 0 {
		p.Max = 4096
	}
	if len(p.seen) >= p.Max {
		for key := range p.seen {
			delete(p.seen, key)
			break
		}
	}
	p.seen[id] = now
	return true
}

func (p *WebhookProcessor) unclaim(id string) { p.mu.Lock(); delete(p.seen, id); p.mu.Unlock() }
