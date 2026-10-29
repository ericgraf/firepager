package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/firepager/backend/models"
)

func TestInviteEndpointsRequireAuthentication(t *testing.T) {
	r := SetupRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/invites", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated invite creation to be rejected, got %d", res.Code)
	}
}

func TestAdminInviteEndpointRequiresAdminRole(t *testing.T) {
	r := SetupRouter()
	user := testUserForInviteAuth()
	token, err := generateToken(user)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/invites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin invite listing to be rejected, got %d", res.Code)
	}
}

func TestReceiverOverviewRequiresAdminRole(t *testing.T) {
	r := SetupRouter()
	user := testUserForInviteAuth()
	token, err := generateToken(user)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/receivers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin receiver overview to be rejected, got %d", res.Code)
	}
}

func testUserForInviteAuth() *models.User {
	return &models.User{ID: 42, Email: "user@example.com", Role: "user"}
}
