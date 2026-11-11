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

func TestBuildBaileysTextRequestNormalizesRecipient(t *testing.T) {
	request, err := buildBaileysTextRequest("+1 (212) 555-0100", "verification")
	if err != nil {
		t.Fatal(err)
	}
	if request.To != "+12125550100" || request.Template.Components[0].Parameters[0].Text != "verification" {
		t.Fatalf("unexpected Baileys text request: %+v", request)
	}
	if _, err := buildBaileysTextRequest("2125550100", "verification"); err == nil {
		t.Fatal("expected local number without country code to be rejected")
	}
}

func TestGetBaileysPairingStatusUsesBridgeAuthAndReturnsQR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer bridge-secret" {
			t.Error("pairing request was missing bridge authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"connection":"pairing","qr":"data:image/png;base64,abc"}`))
	}))
	defer server.Close()
	cfg := BaileysConfig{Enabled: true, Endpoint: server.URL, Token: "bridge-secret"}
	status, err := GetBaileysPairingStatus(context.Background(), cfg, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if status.Connection != "pairing" || status.QR != "data:image/png;base64,abc" {
		t.Fatalf("unexpected pairing response: %+v", status)
	}
}

func TestResetBaileysPairingUsesAuthenticatedBridge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pairing/reset" {
			t.Errorf("unexpected reset request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer bridge-secret" {
			t.Error("reset request was missing bridge authorization")
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	cfg := BaileysConfig{Enabled: true, Endpoint: server.URL, Token: "bridge-secret"}
	if err := ResetBaileysPairing(context.Background(), cfg, server.Client()); err != nil {
		t.Fatal(err)
	}
}
