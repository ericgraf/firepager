package api

import (
	"net/http"

	"github.com/firepager/backend/db"
	"github.com/firepager/backend/models"
	"github.com/firepager/backend/notifications"
	"github.com/gin-gonic/gin"
)

// GET /api/profile/subscriptions returns the authenticated user's normalized
// alert rules and channels. PUT replaces the complete document atomically.
func GetSubscriptionSettingsHandler(c *gin.Context) {
	settings, err := db.GetSubscriptionSettings(c.MustGet("user_id").(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load subscriptions"})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func UpdateSubscriptionSettingsHandler(c *gin.Context) {
	var settings models.SubscriptionSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := db.NormalizeAndValidateSubscriptionSettings(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := db.SaveSubscriptionSettings(c.MustGet("user_id").(int), &settings); err != nil {
		if err == db.ErrWhatsAppNotEligible {
			c.JSON(http.StatusConflict, gin.H{"error": "WhatsApp requires a verified phone number and consent"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save subscriptions"})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func GetSubscriptionCatalogHandler(c *gin.Context) {
	catalog, err := db.GetSubscriptionCatalogs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load subscription catalog"})
		return
	}
	configs, err := db.GetConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load notification provider catalog"})
		return
	}
	configValues := make(map[string]string, len(configs))
	for _, config := range configs {
		configValues[config.Key] = config.Value
	}
	user, err := db.GetUserByID(c.MustGet("user_id").(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load notification provider catalog"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"stations":   catalog["stations"],
		"call_types": catalog["call_types"],
		"channels":   notifications.BuildChannelCatalog(configValues, user),
	})
}

// GetNotificationChannelCatalogHandler is a focused alias for clients that do
// not need the station and call-type subscription dimensions.
func GetNotificationChannelCatalogHandler(c *gin.Context) {
	configs, err := db.GetConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load notification provider catalog"})
		return
	}
	configValues := make(map[string]string, len(configs))
	for _, config := range configs {
		configValues[config.Key] = config.Value
	}
	user, err := db.GetUserByID(c.MustGet("user_id").(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load notification provider catalog"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"channels": notifications.BuildChannelCatalog(configValues, user)})
}
