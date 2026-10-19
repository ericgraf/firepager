package notifications

import (
	"strings"

	"github.com/firepager/backend/models"
)

// channelDefinition is deliberately separate from the sender switch. A
// catalog entry is not a promise that a provider can deliver messages.
type channelDefinition struct {
	id                  string
	label               string
	deliveryImplemented bool
	configured          func(map[string]string, *models.User) bool
	enabled             func(map[string]string) bool
}

var channelDefinitions = []channelDefinition{
	{"sms", "SMS", true, smsConfigured, func(c map[string]string) bool { return configTruthy(c["SMS_GATEWAY_ENABLED"]) }},
	{"email", "Email", true, func(c map[string]string, u *models.User) bool {
		return u != nil && strings.TrimSpace(u.Email) != "" && providerConfigured(c, "EMAIL")
	}, genericEnabled("EMAIL")},
	// push is retained as a legacy subscription identifier. web_push is the
	// explicit provider-backed Web Push adapter.
	{"push", "Push (legacy)", true, genericConfigured("PUSH"), genericEnabled("PUSH")},
	{"whatsapp", "WhatsApp", false, whatsappConfigured, func(c map[string]string) bool { return configTruthy(c["WHATSAPP_GATEWAY_ENABLED"]) }},
	{"telegram", "Telegram", true, genericConfigured("TELEGRAM"), genericEnabled("TELEGRAM")},
	{"signal", "Signal", true, genericConfigured("SIGNAL"), genericEnabled("SIGNAL")},
	{"pushover", "Pushover", true, func(_ map[string]string, u *models.User) bool {
		return u != nil && strings.TrimSpace(u.PushoverUserKey) != "" && strings.TrimSpace(u.PushoverAppToken) != ""
	}, alwaysEnabled},
	{"ntfy", "ntfy", true, genericConfigured("NTFY"), genericEnabled("NTFY")},
	{"web_push", "Web Push", true, genericConfigured("WEB_PUSH"), genericEnabled("WEB_PUSH")},
	{"discord", "Discord", true, genericConfigured("DISCORD"), genericEnabled("DISCORD")},
	{"slack", "Slack", true, genericConfigured("SLACK"), genericEnabled("SLACK")},
	{"teams", "Microsoft Teams", true, genericConfigured("TEAMS"), genericEnabled("TEAMS")},
	{"voice", "Voice", true, genericConfigured("VOICE"), genericEnabled("VOICE")},
}

func alwaysEnabled(_ map[string]string) bool { return true }

func genericEnabled(id string) func(map[string]string) bool {
	return func(config map[string]string) bool { return configTruthy(providerConfigValue(config, id+"_ENABLED")) }
}

func genericConfigured(id string) func(map[string]string, *models.User) bool {
	return func(config map[string]string, _ *models.User) bool {
		// Configured is an inventory signal (a token or endpoint exists). The
		// stricter HTTP adapter check below controls Usable.
		return providerConfigValue(config, id+"_URL") != "" || providerConfigValue(config, id+"_TOKEN") != ""
	}
}

func providerConfigured(config map[string]string, id string) bool {
	c := ProviderConfigFromMap(config, id)
	return c.Configured()
}

func smsConfigured(config map[string]string, _ *models.User) bool {
	return configTruthy(config["SMS_GATEWAY_ENABLED"]) && strings.EqualFold(strings.TrimSpace(config["SMS_PROVIDER"]), "twilio") &&
		strings.TrimSpace(config["TWILIO_ACCOUNT_SID"]) != "" && strings.TrimSpace(config["TWILIO_AUTH_TOKEN"]) != "" && strings.TrimSpace(config["TWILIO_FROM_NUMBER"]) != ""
}

func whatsappConfigured(config map[string]string, _ *models.User) bool {
	return configTruthy(config["WHATSAPP_GATEWAY_ENABLED"]) && strings.TrimSpace(firstNonEmpty(config["META_WHATSAPP_ACCESS_TOKEN"], config["WHATSAPP_ACCESS_TOKEN"])) != "" &&
		strings.TrimSpace(firstNonEmpty(config["META_WHATSAPP_PHONE_NUMBER_ID"], config["WHATSAPP_PHONE_NUMBER_ID"])) != ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// BuildChannelCatalog never performs provider calls. A provider is usable only
// when it is configured, explicitly verified (ID_VERIFIED=true), enabled, and
// has an implemented delivery adapter.
func BuildChannelCatalog(config map[string]string, user *models.User) []models.NotificationChannelCatalog {
	result := make([]models.NotificationChannelCatalog, 0, len(channelDefinitions))
	for _, definition := range channelDefinitions {
		configured := definition.configured(config, user)
		verified := configured && configTruthy(config[strings.ToUpper(definition.id)+"_VERIFIED"])
		enabled := definition.enabled(config)
		ready := providerReady(config, definition.id, configured)
		result = append(result, models.NotificationChannelCatalog{
			ID: definition.id, Label: definition.label, Supported: true,
			Configured: configured, Verified: verified, Disabled: !enabled,
			DeliveryImplemented: definition.deliveryImplemented,
			Usable:              configured && verified && enabled && definition.deliveryImplemented && ready,
		})
	}
	return result
}

func providerReady(config map[string]string, id string, configured bool) bool {
	if !configured {
		return false
	}
	switch id {
	case "email", "push", "telegram", "signal", "ntfy", "web_push", "discord", "slack", "teams", "voice":
		return providerConfigured(config, id)
	default:
		return true
	}
}
