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

func TestMatchesSubscriptionRuleUsesStationEntities(t *testing.T) {
	message := Message{Stations: []string{"82", "81", "46", "60"}, CallType: "Structure Fire"}
	if !MatchesSubscriptionRule(message, SubscriptionRule{Station: "46", Enabled: true}) {
		t.Fatal("expected matching individual station entity to match")
	}
	if MatchesSubscriptionRule(message, SubscriptionRule{Station: "4", Enabled: true}) {
		t.Fatal("expected partial station code not to match")
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
