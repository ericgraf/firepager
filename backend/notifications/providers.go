package notifications

// Provider-neutral HTTP notification adapters. Provider-specific payloads,
// templates, and credential validation remain the responsibility of the
// configured endpoint. Nothing in this file sends unless ENABLED is explicit.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultProviderTimeout  = 10 * time.Second
	defaultProviderAttempts = 3
)

type NotificationRequest struct {
	Provider  string            `json:"provider"`
	Recipient string            `json:"recipient,omitempty"`
	Subject   string            `json:"subject,omitempty"`
	Body      string            `json:"body"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type NotificationProvider interface {
	Send(context.Context, NotificationRequest) error
}

type ProviderConfig struct {
	Name     string
	Enabled  bool
	Verified bool
	Endpoint string
	Token    string
	Timeout  time.Duration
	Attempts int
}

// ProviderConfigFromMap reads the intentionally boring, shared adapter
// settings. Secrets may come from the database or environment before this
// function is called; they are never included in errors.
func ProviderConfigFromMap(values map[string]string, name string) ProviderConfig {
	prefix := strings.ToUpper(strings.TrimSpace(name))
	timeout := defaultProviderTimeout
	if raw := providerConfigValue(values, prefix+"_TIMEOUT_SECONDS"); raw != "" {
		var seconds int
		if _, err := fmt.Sscanf(raw, "%d", &seconds); err == nil && seconds > 0 && seconds <= 120 {
			timeout = time.Duration(seconds) * time.Second
		}
	}
	return ProviderConfig{
		Name: name, Enabled: configTruthy(providerConfigValue(values, prefix+"_ENABLED")),
		Verified: configTruthy(providerConfigValue(values, prefix+"_VERIFIED")),
		Endpoint: providerConfigValue(values, prefix+"_URL"),
		Token:    providerConfigValue(values, prefix+"_TOKEN"),
		Timeout:  timeout, Attempts: defaultProviderAttempts,
	}
}

func providerConfigValue(values map[string]string, key string) string {
	if value := strings.TrimSpace(values[key]); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv(key))
}

func validEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Configured reports the minimum credentials for the named adapter. Webhook
// providers use the URL itself as their credential; API providers require a
// token as well. This deliberately does not infer usable configuration.
func (c ProviderConfig) Configured() bool {
	if !validEndpoint(c.Endpoint) {
		return false
	}
	switch strings.ToLower(c.Name) {
	case "discord", "slack", "teams":
		return true
	default:
		return c.Token != ""
	}
}

func (c ProviderConfig) Usable() bool { return c.Enabled && c.Verified && c.Configured() }

type HTTPProvider struct {
	Config ProviderConfig
	Client *http.Client
}

func NewHTTPProvider(config ProviderConfig) *HTTPProvider {
	timeout := config.Timeout
	if timeout <= 0 || timeout > 120*time.Second {
		timeout = defaultProviderTimeout
	}
	return &HTTPProvider{Config: config, Client: &http.Client{Timeout: timeout}}
}

func (p *HTTPProvider) Send(ctx context.Context, request NotificationRequest) error {
	if p == nil || !p.Config.Usable() {
		return errors.New("notification provider is not configured")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return errors.New("notification request could not be encoded")
	}
	attempts := p.Config.Attempts
	if attempts < 1 || attempts > 3 {
		attempts = defaultProviderAttempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Config.Endpoint, bytes.NewReader(payload))
		if err != nil {
			return errors.New("notification request could not be created")
		}
		req.Header.Set("Content-Type", "application/json")
		if p.Config.Token != "" {
			req.Header.Set("Authorization", "Bearer "+p.Config.Token)
		}
		resp, err := p.Client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			class := ClassifyRetry(resp.StatusCode, nil)
			if class != RetryTransient || attempt == attempts-1 {
				return RedactProviderErrorFor(p.Config.Name, resp.StatusCode)
			}
		} else {
			if ClassifyRetry(0, err) != RetryTransient || attempt == attempts-1 {
				return RedactProviderErrorFor(p.Config.Name, 0)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 50 * time.Millisecond):
		}
	}
	return errors.New("notification provider request failed")
}

func RedactProviderErrorFor(provider string, status int) error {
	name := strings.ToLower(strings.TrimSpace(provider))
	if name == "" {
		name = "notification"
	}
	if status > 0 {
		return fmt.Errorf("%s provider request failed (status %d)", name, status)
	}
	return fmt.Errorf("%s provider request failed", name)
}

type MockProvider struct {
	Requests []NotificationRequest
	Err      error
}

func (m *MockProvider) Send(_ context.Context, request NotificationRequest) error {
	if m == nil {
		return errors.New("nil mock provider")
	}
	m.Requests = append(m.Requests, request)
	return m.Err
}
