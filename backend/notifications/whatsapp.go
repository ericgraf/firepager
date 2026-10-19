package notifications

// This file contains the provider-neutral WhatsApp boundary.  It deliberately
// does not contain an HTTP sender: production wiring must provide and review a
// provider implementation explicitly.  In particular, templates are the only
// supported message shape; free-form message text is never sent by this API.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

var (
	ErrWhatsAppDisabled = errors.New("whatsapp is disabled")
	ErrWhatsAppConsent  = errors.New("whatsapp consent is required")
	ErrInvalidE164      = errors.New("invalid E.164 phone number")
)

// MetaCloudConfig contains non-secret settings needed to construct a Meta
// Cloud API request. AccessToken is retained for the eventual provider, but is
// never included in a request payload or an error.
type MetaCloudConfig struct {
	Enabled       bool
	AccessToken   string
	PhoneNumberID string
	APIVersion    string
	TemplateName  string
	LanguageCode  string
}

// ParseMetaCloudConfig parses configuration without enabling delivery unless
// WHATSAPP_GATEWAY_ENABLED is explicitly truthy.
func ParseMetaCloudConfig(values map[string]string) MetaCloudConfig {
	get := func(keys ...string) string {
		for _, key := range keys {
			if value := strings.TrimSpace(values[key]); value != "" {
				return value
			}
		}
		return ""
	}
	return MetaCloudConfig{
		Enabled:       configTruthy(get("WHATSAPP_GATEWAY_ENABLED", "META_WHATSAPP_ENABLED")),
		AccessToken:   get("META_WHATSAPP_ACCESS_TOKEN", "WHATSAPP_ACCESS_TOKEN"),
		PhoneNumberID: get("META_WHATSAPP_PHONE_NUMBER_ID", "WHATSAPP_PHONE_NUMBER_ID"),
		APIVersion:    get("META_WHATSAPP_API_VERSION", "WHATSAPP_API_VERSION"),
		TemplateName:  get("META_WHATSAPP_TEMPLATE_NAME", "WHATSAPP_TEMPLATE_NAME"),
		LanguageCode:  get("META_WHATSAPP_TEMPLATE_LANGUAGE", "WHATSAPP_TEMPLATE_LANGUAGE"),
	}
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
