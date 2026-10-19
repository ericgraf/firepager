package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/firepager/backend/db"
	"github.com/firepager/backend/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var jwtSecret = []byte(os.Getenv("JWT_SECRET"))

func init() {
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("default_secret_for_development_only")
	}
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RegisterRequest struct {
	Email       string   `json:"email" binding:"required"`
	Password    string   `json:"password" binding:"required"`
	Name        string   `json:"name" binding:"required"`
	PhoneNumber string   `json:"phone_number" binding:"required"`
	InviteCode  string   `json:"invite_code" binding:"required"`
	Capcodes    []string `json:"capcodes"`
}

func LoginHandler(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := db.GetUserByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	token, err := generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  user,
	})
}

func RegisterHandler(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
		return
	}

	user := &models.User{
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Name:         req.Name,
		PhoneNumber:  req.PhoneNumber,
		Capcodes:     req.Capcodes,
		Role:         "user",
	}

	if err := db.AddUserWithInvite(user, req.InviteCode); err != nil {
		// Do not disclose whether a code exists, has expired, or was already used.
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid, expired, revoked, or already used invite code"})
		return
	}

	token, err := generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"token": token,
		"user":  user,
	})
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header format must be Bearer {token}"})
			c.Abort()
			return
		}

		tokenString := parts[1]
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			c.Abort()
			return
		}

		c.Set("user_id", int(claims["user_id"].(float64)))
		c.Set("role", claims["role"].(string))
		c.Next()
	}
}

func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func generateToken(user *models.User) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})

	return token.SignedString(jwtSecret)
}

func CreateInviteHandler(c *gin.Context) {
	creatorID, _ := c.Get("user_id")

	// Generate random code
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate invite code"})
		return
	}
	code := hex.EncodeToString(b)

	invite := &models.Invite{
		Code:      code,
		CreatedBy: creatorID.(int),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}

	if err := db.CreateInvite(invite); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invite in database"})
		return
	}

	c.JSON(http.StatusCreated, invite)
}

type UpdateProfileRequest struct {
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	PhoneNumber       string   `json:"phone_number"`
	Password          string   `json:"password"`
	Capcodes          []string `json:"capcodes"`
	Vehicles          []string `json:"vehicles"`
	Stations          []string `json:"stations"`
	CallTypes         []string `json:"call_types"`
	NotificationTypes []string `json:"notification_types"`
}

func GetInvitesHandler(c *gin.Context) {
	invites, err := db.GetInvites()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, invites)
}

func GetMyInvitesHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	invites, err := db.GetInvitesForUser(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load invites"})
		return
	}
	c.JSON(http.StatusOK, invites)
}

func RevokeInviteHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invite id"})
		return
	}
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	if err := db.RevokeInvite(id, userID.(int), role == "admin"); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invite not found or cannot be revoked"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke invite"})
		return
	}
	c.Status(http.StatusNoContent)
}

func GetProfileHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	user, err := db.GetUserByID(userID.(int))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func UpdateProfileHandler(c *gin.Context) {
	userID, _ := c.Get("user_id")
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := db.GetUserByID(userID.(int))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Name != "" {
		user.Name = req.Name
	}
	if req.PhoneNumber != "" {
		user.PhoneNumber = req.PhoneNumber
	}
	if req.Capcodes != nil {
		user.Capcodes = req.Capcodes
	}
	if req.Vehicles != nil {
		user.Vehicles = req.Vehicles
	}
	if req.Stations != nil {
		user.Stations = req.Stations
	}
	if req.CallTypes != nil {
		user.CallTypes = req.CallTypes
	}
	if req.NotificationTypes != nil {
		user.NotificationTypes = req.NotificationTypes
	}

	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		user.PasswordHash = string(hashedPassword)
	} else {
		user.PasswordHash = "" // Don't update password if not provided
	}

	if err := db.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, user)
}
