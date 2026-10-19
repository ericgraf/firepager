package db

import (
	"reflect"
	"testing"

	"github.com/firepager/backend/models"
)

func TestNormalizeUserArrays(t *testing.T) {
	user := &models.User{}

	normalizeUserArrays(user)

	if user.Capcodes == nil || user.Vehicles == nil || user.Stations == nil || user.CallTypes == nil || user.NotificationTypes == nil {
		t.Fatal("expected all user array fields to be initialized")
	}

	if !reflect.DeepEqual(user.Capcodes, []string{}) {
		t.Fatalf("expected empty capcodes slice, got %#v", user.Capcodes)
	}
}
