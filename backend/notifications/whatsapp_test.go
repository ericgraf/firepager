package notifications

import (
	"context"
	"errors"
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
