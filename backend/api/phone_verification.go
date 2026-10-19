package api

import (
	"errors"
	"net/http"

	"github.com/firepager/backend/db"
	"github.com/gin-gonic/gin"
)

type phoneVerificationCodeRequest struct {
	Code string `json:"code" binding:"required"`
}
type whatsappConsentRequest struct {
	PolicyVersion string `json:"policy_version" binding:"required"`
}

func RequestPhoneVerificationHandler(c *gin.Context) {
	if err := db.CreatePhoneVerificationChallenge(c.MustGet("user_id").(int)); err != nil {
		if errors.Is(err, db.ErrVerificationRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Verification request rate limit exceeded"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create verification challenge"})
		return
	}
	// Never include the OTP in a response. Delivery is intentionally owned by
	// the notification boundary and is not exposed through this API.
	c.JSON(http.StatusAccepted, gin.H{"status": "verification_sent", "expires_in": 600})
}

func ConfirmPhoneVerificationHandler(c *gin.Context) {
	var req phoneVerificationCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A verification code is required"})
		return
	}
	if err := db.ConfirmPhoneVerification(c.MustGet("user_id").(int), req.Code); err != nil {
		if errors.Is(err, db.ErrVerificationInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired verification code"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to confirm phone number"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "verified"})
}

func GrantWhatsAppConsentHandler(c *gin.Context) {
	var req whatsappConsentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A consent policy version is required"})
		return
	}
	if err := db.SetWhatsAppConsent(c.MustGet("user_id").(int), true, req.PolicyVersion); err != nil {
		if err.Error() == "consent policy version is required" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save WhatsApp consent"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "consented", "policy_version": req.PolicyVersion})
}

func RevokeWhatsAppConsentHandler(c *gin.Context) {
	var req whatsappConsentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A consent policy version is required"})
		return
	}
	if err := db.SetWhatsAppConsent(c.MustGet("user_id").(int), false, req.PolicyVersion); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke WhatsApp consent"})
		return
	}
	c.Status(http.StatusNoContent)
}
