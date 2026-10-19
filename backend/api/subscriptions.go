package api

import (
	"net/http"

	"github.com/firepager/backend/db"
	"github.com/firepager/backend/models"
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
	c.JSON(http.StatusOK, catalog)
}
