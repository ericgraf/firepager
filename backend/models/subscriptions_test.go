package models

import "testing"

func TestMatchesSubscriptionRule(t *testing.T) {
	message := Message{Station: "12, 14", CallType: "Structure Fire"}
	if !MatchesSubscriptionRule(message, SubscriptionRule{Station: "14", CallType: "structure fire", Enabled: true}) {
		t.Fatal("expected matching station and call type")
	}
	if MatchesSubscriptionRule(message, SubscriptionRule{Station: "99", CallType: "", Enabled: true}) {
		t.Fatal("unexpected station match")
	}
	if MatchesSubscriptionRule(message, SubscriptionRule{Station: "12", Enabled: false}) {
		t.Fatal("disabled rule matched")
	}
}

func TestMatchesSubscription(t *testing.T) {
	message := Message{Station: "12", CallType: "Medical"}
	settings := SubscriptionSettings{Enabled: true, Rules: []SubscriptionRule{{CallType: "Medical", Enabled: true}}}
	if !MatchesSubscription(message, settings) {
		t.Fatal("expected subscription match")
	}
	settings.Enabled = false
	if MatchesSubscription(message, settings) {
		t.Fatal("disabled subscription matched")
	}
}
