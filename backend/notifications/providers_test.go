package notifications

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderConfigRequiresExplicitGatesAndCredentials(t *testing.T) {
	c := ProviderConfigFromMap(map[string]string{
		"TELEGRAM_URL":      "https://example.invalid/send",
		"TELEGRAM_TOKEN":    "secret",
		"TELEGRAM_ENABLED":  "true",
		"TELEGRAM_VERIFIED": "true",
	}, "telegram")
	if !c.Configured() || !c.Usable() {
		t.Fatalf("expected complete provider config: %+v", c)
	}
	c.Enabled = false
	if c.Usable() {
		t.Fatal("disabled provider was usable")
	}
	if ProviderConfigFromMap(map[string]string{"DISCORD_ENABLED": "true"}, "discord").Configured() {
		t.Fatal("missing webhook URL was accepted")
	}
}

func TestHTTPProviderUsesMockServerAndRedactsFailures(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	c := ProviderConfig{Name: "telegram", Enabled: true, Verified: true, Endpoint: server.URL, Token: "secret-token", Attempts: 1}
	err := NewHTTPProvider(c).Send(context.Background(), NotificationRequest{Provider: "telegram", Body: "private alert"})
	if err == nil || strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "private") {
		t.Fatalf("unsafe or missing error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("token was not sent as auth header: %q", gotAuth)
	}
}

func TestMockProviderIsDeterministicAndNeverNetworked(t *testing.T) {
	mock := &MockProvider{}
	request := NotificationRequest{Provider: "voice", Recipient: "+15550000000", Body: "test"}
	if err := mock.Send(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(mock.Requests) != 1 || mock.Requests[0].Recipient != request.Recipient {
		t.Fatalf("unexpected mock requests: %+v", mock.Requests)
	}
}
