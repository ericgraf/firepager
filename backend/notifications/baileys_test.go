package notifications

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBaileysConfigRequiresExplicitProviderAndGateway(t *testing.T) {
	cfg := ParseBaileysConfig(map[string]string{"WHATSAPP_BRIDGE_URL": "http://bridge", "WHATSAPP_BRIDGE_AUTH_TOKEN": "secret"})
	if cfg.Enabled {
		t.Fatal("Baileys must be disabled without explicit provider and gateway")
	}
	cfg = ParseBaileysConfig(map[string]string{"WHATSAPP_PROVIDER": "baileys", "WHATSAPP_GATEWAY_ENABLED": "true", "WHATSAPP_BRIDGE_URL": "http://bridge", "WHATSAPP_BRIDGE_AUTH_TOKEN": "secret"})
	if !cfg.Enabled {
		t.Fatal("explicit Baileys configuration was not enabled")
	}
}

func TestBaileysSenderRedactsProviderFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "token=secret private message", http.StatusBadGateway)
	}))
	defer server.Close()
	sender := NewBaileysSender(BaileysConfig{Enabled: true, Endpoint: server.URL, Token: "secret"}, server.Client())
	req, err := BuildMetaCloudTemplateRequest(MetaCloudConfig{TemplateName: "bridge", LanguageCode: "en"}, "+12125550100", []string{"private message"})
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(context.Background(), req)
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") {
		t.Fatalf("unsafe provider error: %v", err)
	}
}
