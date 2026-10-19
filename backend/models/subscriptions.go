package models

import "strings"

// MatchesSubscriptionRule applies the same wildcard semantics used by the
// subscription API. Message stations may contain a comma-separated list.
func MatchesSubscriptionRule(message Message, rule SubscriptionRule) bool {
	if !rule.Enabled {
		return false
	}
	stationMatch := strings.TrimSpace(rule.Station) == ""
	for _, station := range strings.Split(message.Station, ",") {
		if strings.EqualFold(strings.TrimSpace(station), strings.TrimSpace(rule.Station)) {
			stationMatch = true
		}
	}
	callTypeMatch := strings.TrimSpace(rule.CallType) == "" || strings.EqualFold(strings.TrimSpace(message.CallType), strings.TrimSpace(rule.CallType))
	return stationMatch && callTypeMatch
}

// MatchesSubscription returns whether an enabled subscription would receive a
// message. Notification delivery is deliberately outside this helper.
func MatchesSubscription(message Message, settings SubscriptionSettings) bool {
	if !settings.Enabled {
		return false
	}
	for _, rule := range settings.Rules {
		if MatchesSubscriptionRule(message, rule) {
			return true
		}
	}
	return false
}
