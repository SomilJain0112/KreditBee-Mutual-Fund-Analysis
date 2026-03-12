package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/services"
)

type FundHandler struct {
	fundService      *services.FundService
	analyticsService *services.AnalyticsService
	logger           *zap.Logger
}

func NewFundHandler(fundService *services.FundService, analyticsService *services.AnalyticsService, logger *zap.Logger) *FundHandler {
	return &FundHandler{
		fundService:      fundService,
		analyticsService: analyticsService,
		logger:           logger,
	}
}

func (h *FundHandler) ListFunds(c *gin.Context) {
	funds, err := h.fundService.ListFunds(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list funds", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve funds",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Success: true,
		Data:    funds,
	})
}

func (h *FundHandler) GetFund(c *gin.Context) {
	codeStr := c.Param("code")
	code, err := strconv.ParseInt(codeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_parameter",
			Message: "Invalid scheme code",
		})
		return
	}

	fund, err := h.fundService.GetFund(c.Request.Context(), code)
	if err != nil {
		h.logger.Error("failed to get fund", zap.Int64("code", code), zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve fund",
		})
		return
	}

	if fund == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Fund not found",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Success: true,
		Data:    fund,
	})
}

func (h *FundHandler) GetFundAnalytics(c *gin.Context) {
	codeStr := c.Param("code")
	code, err := strconv.ParseInt(codeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_parameter",
			Message: "Invalid scheme code",
		})
		return
	}

	analytics, err := h.fundService.GetFundAnalytics(c.Request.Context(), code)
	if err != nil {
		h.logger.Error("failed to get fund analytics", zap.Int64("code", code), zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve analytics",
		})
		return
	}

	if analytics == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Fund not found",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Success: true,
		Data:    analytics,
	})
}

func (h *FundHandler) RankFunds(c *gin.Context) {
	windowCode := c.DefaultQuery("window", "1Y")
	sortBy := c.DefaultQuery("sort_by", "cagr")
	limitStr := c.DefaultQuery("limit", "10")

	validWindows := map[string]bool{"1Y": true, "3Y": true, "5Y": true, "10Y": true}
	if !validWindows[windowCode] {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_parameter",
			Message: "Invalid window code. Valid values: 1Y, 3Y, 5Y, 10Y",
		})
		return
	}

	validSortBy := map[string]bool{
		"cagr": true, "rolling_return": true, "max_drawdown": true,
		"sharpe_ratio": true, "volatility": true,
	}
	if !validSortBy[sortBy] {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_parameter",
			Message: "Invalid sort_by. Valid values: cagr, rolling_return, max_drawdown, sharpe_ratio, volatility",
		})
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 10
	}

	rankedFunds, err := h.analyticsService.RankFunds(c.Request.Context(), windowCode, sortBy, limit)
	if err != nil {
		h.logger.Error("failed to rank funds", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to rank funds",
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Success: true,
		Data: gin.H{
			"window":  windowCode,
			"sort_by": sortBy,
			"count":   len(rankedFunds),
			"funds":   rankedFunds,
		},
	})
}

type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
