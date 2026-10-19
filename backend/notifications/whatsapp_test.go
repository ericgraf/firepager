package notifications

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseMetaCloudConfigDisabledByDefault(t *testing.T) {
	cfg := ParseMetaCloudConfig(map[string]string{
		"META_WHATSAPP_ACCESS_TOKEN":  "secret-token",
		"META_WHATSAPP_TEMPLATE_NAME": "fire_alert",
	})
	if cfg.Enabled {
		t.Fatal("configuration must be disabled unless explicitly enabled")
	}
	if cfg.AccessToken != "secret-token" {
		t.Fatal("access token was not parsed")
	}
}

func TestMetaCloudSenderUsesTemplateAndRetriesTransientFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing authorization")
		}
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	cfg := MetaCloudConfig{Enabled: true, AccessToken: "secret", PhoneNumberID: "id", Endpoint: server.URL, Attempts: 2}
	sender := NewMetaCloudSender(cfg, server.Client())
	req, err := BuildMetaCloudTemplateRequest(MetaCloudConfig{TemplateName: "alert", LanguageCode: "en"}, "+12125550100", nil)
	if err != nil || sender.Send(context.Background(), req) != nil {
		t.Fatalf("sender failed: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected retry, got %d requests", attempts)
	}
}

func TestMetaWebhookSignatureAndDeduplication(t *testing.T) {
	body := []byte(`{"entry":[{"changes":[{"value":{"statuses":[{"id":"wamid.1","status":"delivered"}]}}]}]}`)
	secret := "app-secret"
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	if !VerifyMetaSignature(body, "sha256="+fmt.Sprintf("%x", h.Sum(nil)), secret) {
		t.Fatal("valid signature rejected")
	}
	if VerifyMetaSignature(body, "sha256=bad", secret) {
		t.Fatal("invalid signature accepted")
	}
	seen := 0
	p := NewWebhookProcessor(func(context.Context, MetaStatus) error { seen++; return nil })
	if err := p.Process(context.Background(), body); err != nil {
		t.Fatal(err)
	}
	if err := p.Process(context.Background(), body); err != nil {
		t.Fatal(err)
	}
	if seen != 1 {
		t.Fatalf("duplicate status handled %d times", seen)
	}
}

func TestBuildMetaCloudTemplateRequestIsTemplateOnly(t *testing.T) {
	cfg := MetaCloudConfig{TemplateName: "fire_alert", LanguageCode: "en_US"}
	request, err := BuildMetaCloudTemplateRequest(cfg, "+1 (212) 555-0100", []string{"Engine 1", "Main St"})
	if err != nil {
		t.Fatal(err)
	}
	if request.Type != "template" || request.MessagingProduct != "whatsapp" {
		t.Fatalf("unexpected request type: %+v", request)
	}
	if len(request.Template.Components) != 1 || len(request.Template.Components[0].Parameters) != 2 {
		t.Fatalf("template parameters missing: %+v", request.Template)
	}
	payload, err := MarshalMetaCloudTemplateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"text"`) && !strings.Contains(string(payload), `"type":"text"`) {
		t.Fatal("payload contains an unsupported free-form text field")
	}
}

func TestAdapterGatesDisabledAndConsent(t *testing.T) {
	mock := NewMockWhatsAppProvider()
	adapter := WhatsAppAdapter{Config: MetaCloudConfig{TemplateName: "x", LanguageCode: "en"}, Provider: mock}
	if !errors.Is(adapter.SendTemplate(context.Background(), "+12125550100", true, nil), ErrWhatsAppDisabled) {
		t.Fatal("disabled adapter attempted delivery")
	}
	adapter.Config.Enabled = true
	if !errors.Is(adapter.SendTemplate(context.Background(), "+12125550100", false, nil), ErrWhatsAppConsent) {
		t.Fatal("missing consent was not rejected")
	}
	if len(mock.Requests) != 0 {
		t.Fatal("provider was called despite safety gates")
	}
}

func TestNormalizeE164AndMock(t *testing.T) {
	number, err := ConsentGate("+1 212-555-0100", true)
	if err != nil || number != "+12125550100" {
		t.Fatalf("unexpected normalized number %q: %v", number, err)
	}
	if _, err := NormalizeE164("212-555-0100"); !errors.Is(err, ErrInvalidE164) {
		t.Fatal("local number was accepted")
	}
	mock := NewMockWhatsAppProvider()
	adapter := WhatsAppAdapter{Config: MetaCloudConfig{Enabled: true, TemplateName: "x", LanguageCode: "en"}, Provider: mock}
	if err := adapter.SendTemplate(context.Background(), number, true, []string{"alert"}); err != nil {
		t.Fatal(err)
	}
	if len(mock.Requests) != 1 || mock.Requests[0].To != number {
		t.Fatalf("mock did not record request: %+v", mock.Requests)
	}
}

func TestRetryClassificationAndRedaction(t *testing.T) {
	if ClassifyRetry(429, nil) != RetryTransient || ClassifyRetry(503, nil) != RetryTransient {
		t.Fatal("expected transient status")
	}
	if ClassifyRetry(400, nil) != RetryPermanent || ClassifyRetry(200, nil) != RetryNo {
		t.Fatal("unexpected status classification")
	}
	if got := RedactSecret("access-token"); got == "access-token" || got == "" {
		t.Fatal("secret was not redacted")
	}
	if got := RedactProviderError(400).Error(); strings.Contains(got, "token") || strings.Contains(got, "message") {
		t.Fatalf("unsafe provider error: %s", got)
	}
}
