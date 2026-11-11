package api

import (
	"net/http"

	"github.com/firepager/backend/notifications"
	"github.com/gin-gonic/gin"
)

func GetWhatsAppPairingHandler(c *gin.Context) {
	// Pairing is a setup operation, so it can be used before the provider is
	// enabled in the database. Credentials and the bridge endpoint still come
	// only from the backend's protected environment.
	bridge := notifications.ParseBaileysConfig(map[string]string{
		"WHATSAPP_GATEWAY_ENABLED": "true",
		"WHATSAPP_PROVIDER":        "baileys",
	})
	status, err := notifications.GetBaileysPairingStatus(c.Request.Context(), bridge, nil)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "WhatsApp pairing status is unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, status)
}
