package api

import (
	"database/sql"
	"fmt"
	"net/http"

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
			protected.GET("/configs", getConfigs)

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
	metrics, err := db.GetMetrics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, metrics)
}

func getMessages(c *gin.Context) {
	msgs, err := db.GetMessages()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, msgs)
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
