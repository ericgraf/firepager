package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/firepager/backend/models"
)

func TestReceiverWaterfallsRequiresAuthentication(t *testing.T) {
	r := SetupRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/receivers/remote-pi-sdr/waterfalls", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated request to be rejected with 401, got %d", res.Code)
	}
}

func TestReceiverWaterfallsRequiresAdminRole(t *testing.T) {
	r := SetupRouter()
	user := &models.User{ID: 10, Email: "user@example.com", Role: "user"}
	token, err := generateToken(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/receivers/remote-pi-sdr/waterfalls", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin request to be rejected with 403, got %d", res.Code)
	}
}

func TestAdminCanAccessReceiverWaterfalls(t *testing.T) {
	r := SetupRouter()
	admin := &models.User{ID: 1, Email: "admin@example.com", Role: "admin"}
	token, err := generateToken(admin)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/receivers/remote-pi-sdr/waterfalls?limit=12", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected admin request to succeed with 200, got %d: %s", res.Code, res.Body.String())
	}

	var response []models.ReceiverWaterfall
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON array response, got error: %v, body: %s", err, res.Body.String())
	}
	if response == nil {
		t.Fatal("expected non-nil response array")
	}
}
