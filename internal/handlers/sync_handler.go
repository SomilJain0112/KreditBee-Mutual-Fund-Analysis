package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/models"
	"github.com/kreditbee/mf-analytics/internal/services"
)

type SyncHandler struct {
	syncService      *services.SyncService
	analyticsService *services.AnalyticsService
	logger           *zap.Logger
}

func NewSyncHandler(syncService *services.SyncService, analyticsService *services.AnalyticsService, logger *zap.Logger) *SyncHandler {
	return &SyncHandler{
		syncService:      syncService,
		analyticsService: analyticsService,
		logger:           logger,
	}
}

type TriggerSyncRequest struct {
	SyncType          string `json:"sync_type" binding:"required,oneof=backfill incremental"`
	RecalculateMetrics bool   `json:"recalculate_metrics"`
}

func (h *SyncHandler) TriggerSync(c *gin.Context) {
	var req TriggerSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body. sync_type must be 'backfill' or 'incremental'",
		})
		return
	}

	if h.syncService.IsRunning() {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error:   "sync_in_progress",
			Message: "A sync operation is already in progress",
		})
		return
	}

	if err := h.syncService.TriggerSync(c.Request.Context(), req.SyncType); err != nil {
		h.logger.Error("failed to trigger sync", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to trigger sync",
		})
		return
	}

	if req.RecalculateMetrics {
		go func() {
			bgCtx := context.Background()
			if err := h.analyticsService.RecalculateAllAnalytics(bgCtx); err != nil {
				h.logger.Error("failed to recalculate analytics", zap.Error(err))
			}
		}()
	}

	c.JSON(http.StatusAccepted, SuccessResponse{
		Success: true,
		Data: gin.H{
			"message":   "Sync triggered successfully",
			"sync_type": req.SyncType,
		},
	})
}

func (h *SyncHandler) GetSyncStatus(c *gin.Context) {
	status, err := h.syncService.GetSyncStatus(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get sync status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to get sync status",
		})
		return
	}

	var backfillCount, incrementalCount int
	var completedCount, failedCount, pendingCount int
	for _, f := range status.Funds {
		if f.SyncType == models.SyncTypeBackfill {
			backfillCount++
		} else {
			incrementalCount++
		}
		switch f.Status {
		case models.SyncStatusCompleted:
			completedCount++
		case models.SyncStatusFailed:
			failedCount++
		case models.SyncStatusPending, models.SyncStatusInProgress:
			pendingCount++
		}
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Success: true,
		Data: gin.H{
			"is_running":        status.IsRunning,
			"rate_limit_status": status.RateLimitStatus,
			"summary": gin.H{
				"total_funds":       len(status.Funds) / 2,
				"backfill_syncs":    backfillCount,
				"incremental_syncs": incrementalCount,
				"completed":         completedCount,
				"failed":            failedCount,
				"pending":           pendingCount,
			},
			"funds": status.Funds,
		},
	})
}
