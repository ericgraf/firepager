package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/firepager/backend/db"
	"github.com/firepager/backend/models"
	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	{
		api.POST("/login", LoginHandler)
		api.POST("/register", RegisterHandler)

		// Protected routes
		protected := api.Group("/")
		protected.Use(AuthMiddleware())
		{
			protected.GET("/messages", getMessages)
			protected.GET("/users", getUsers)
			protected.POST("/users", addUser)
			protected.GET("/metrics", getMetrics)
			protected.GET("/profile", GetProfileHandler)
			protected.PUT("/profile", UpdateProfileHandler)
			protected.GET("/profile/subscriptions", GetSubscriptionSettingsHandler)
			protected.PUT("/profile/subscriptions", UpdateSubscriptionSettingsHandler)
			protected.GET("/profile/subscriptions/catalog", GetSubscriptionCatalogHandler)
			protected.GET("/configs", getConfigs)
			protected.GET("/invites", GetMyInvitesHandler)
			protected.POST("/invites", CreateInviteHandler)
			protected.DELETE("/invites/:id", RevokeInviteHandler)

			// Admin only routes
			admin := protected.Group("/admin")
			admin.Use(AdminMiddleware())
			{
				admin.GET("/users", getUsers) // Existing but let's make it admin-only if needed, or keep both
				admin.GET("/invites", GetInvitesHandler)
				admin.POST("/invites", CreateInviteHandler)
				admin.DELETE("/messages/:id", deleteMessage)
				admin.PUT("/users/:id/role", updateUserRole)
				admin.PUT("/configs", updateConfigs)
			}
		}
	}

	return r
}

func getConfigs(c *gin.Context) {
	configs, err := db.GetConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, configs)
}

func updateConfigs(c *gin.Context) {
	var config models.SystemConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := db.UpdateConfig(config.Key, config.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func updateUserRole(c *gin.Context) {
	idStr := c.Param("id")
	var req struct {
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var id int
	fmt.Sscanf(idStr, "%d", &id)
	if err := db.UpdateUserRole(id, req.Role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func getMetrics(c *gin.Context) {
	query, err := parseMetricsQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	metrics, err := db.GetMetricsFiltered(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, metrics)
}

func getMessages(c *gin.Context) {
	// Keep the original []Message response for existing clients that do not
	// ask for query options. New clients get bounded keyset pagination.
	if c.Request.URL.RawQuery == "" {
		msgs, err := db.GetMessages()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, msgs)
		return
	}
	query, err := parseMessageQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, err := db.GetMessagesPage(query)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}

func parseTimeParam(c *gin.Context, name string) (*time.Time, error) {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, fmt.Errorf("%s must be RFC3339", name)
	}
	return &t, nil
}

func parseMessageQuery(c *gin.Context) (db.MessageQuery, error) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 {
			return db.MessageQuery{}, fmt.Errorf("limit must be a positive integer")
		}
	}
	from, err := parseTimeParam(c, "from")
	if err != nil {
		return db.MessageQuery{}, err
	}
	to, err := parseTimeParam(c, "to")
	if err != nil {
		return db.MessageQuery{}, err
	}
	if from != nil && to != nil && !from.Before(*to) {
		return db.MessageQuery{}, fmt.Errorf("from must be before to")
	}
	return db.MessageQuery{
		Limit: limit, Cursor: c.Query("cursor"), Type: c.Query("type"),
		Source: c.Query("source"), ClientID: c.Query("client_id"),
		Station: c.Query("station"), CallType: c.Query("call_type"),
		Search: c.Query("q"), From: from, To: to,
	}, nil
}

func parseMetricsQuery(c *gin.Context) (db.MetricsQuery, error) {
	from, err := parseTimeParam(c, "from")
	if err != nil {
		return db.MetricsQuery{}, err
	}
	to, err := parseTimeParam(c, "to")
	if err != nil {
		return db.MetricsQuery{}, err
	}
	if from != nil && to != nil && !from.Before(*to) {
		return db.MetricsQuery{}, fmt.Errorf("from must be before to")
	}
	return db.MetricsQuery{
		Type: c.Query("type"), ClientID: c.Query("client_id"),
		Station: c.Query("station"), CallType: c.Query("call_type"),
		From: from, To: to,
	}, nil
}

func deleteMessage(c *gin.Context) {
	if err := db.DeleteMessage(c.Param("id")); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func getUsers(c *gin.Context) {
	users, err := db.GetUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

func addUser(c *gin.Context) {
	var u models.User
	if err := c.ShouldBindJSON(&u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := db.AddUser(&u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, u)
}
