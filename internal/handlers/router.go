package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/config"
	"github.com/kreditbee/mf-analytics/internal/services"
)

func NewRouter(
	cfg *config.Config,
	fundService *services.FundService,
	syncService *services.SyncService,
	analyticsService *services.AnalyticsService,
	logger *zap.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))
	router.Use(corsMiddleware())

	fundHandler := NewFundHandler(fundService, analyticsService, logger)
	syncHandler := NewSyncHandler(syncService, analyticsService, logger)

	router.GET("/health", healthHandler())

	api := router.Group("/api/v1")
	{
		funds := api.Group("/funds")
		{
			funds.GET("", fundHandler.ListFunds)
			funds.GET("/:code", fundHandler.GetFund)
			funds.GET("/:code/analytics", fundHandler.GetFundAnalytics)
			funds.GET("/rank", fundHandler.RankFunds)
		}

		sync := api.Group("/sync")
		{
			sync.POST("/trigger", syncHandler.TriggerSync)
			sync.GET("/status", syncHandler.GetSyncStatus)
		}
	}

	return router
}

func healthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func requestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
