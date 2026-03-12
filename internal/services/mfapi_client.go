package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/config"
	"github.com/kreditbee/mf-analytics/internal/models"
)

type MFAPIClient struct {
	cfg         config.MFAPIConfig
	rateLimiter *RateLimiter
	httpClient  *http.Client
	logger      *zap.Logger
}

type MFAPIResponse struct {
	Meta struct {
		FundHouse  string `json:"fund_house"`
		SchemeType string `json:"scheme_type"`
		SchemeName string `json:"scheme_name"`
		SchemeCode int64  `json:"scheme_code"`
		ISIN       string `json:"isin_growth"`
	} `json:"meta"`
	Data []struct {
		Date string `json:"date"`
		NAV  string `json:"nav"`
	} `json:"data"`
	Status string `json:"status"`
}

func NewMFAPIClient(cfg config.MFAPIConfig, rateLimiter *RateLimiter, logger *zap.Logger) *MFAPIClient {
	return &MFAPIClient{
		cfg:         cfg,
		rateLimiter: rateLimiter,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

func (c *MFAPIClient) FetchSchemeData(ctx context.Context, schemeCode int64) (*MFAPIResponse, error) {
	if err := c.rateLimiter.Acquire(ctx); err != nil {
		return nil, fmt.Errorf("rate limit error: %w", err)
	}

	url := fmt.Sprintf("%s/mf/%d", c.cfg.BaseURL, schemeCode)
	c.logger.Debug("fetching scheme data", zap.String("url", url))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MF-Analytics/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scheme data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		c.logger.Warn("received 429 from MFAPI, triggering block")
		return nil, ErrRateLimitExceeded
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp MFAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Status == "ERROR" {
		return nil, fmt.Errorf("MFAPI returned error for scheme %d", schemeCode)
	}

	c.logger.Debug("fetched scheme data",
		zap.Int64("scheme_code", schemeCode),
		zap.Int("nav_count", len(apiResp.Data)),
	)

	return &apiResp, nil
}

func (c *MFAPIClient) ParseNAVData(fundID int64, data []struct {
	Date string `json:"date"`
	NAV  string `json:"nav"`
}) ([]models.NAVHistory, error) {
	var navHistory []models.NAVHistory

	for _, d := range data {
		date, err := time.Parse("02-01-2006", d.Date)
		if err != nil {
			date, err = time.Parse("2006-01-02", d.Date)
			if err != nil {
				c.logger.Warn("failed to parse date", zap.String("date", d.Date), zap.Error(err))
				continue
			}
		}

		nav, err := decimal.NewFromString(d.NAV)
		if err != nil {
			c.logger.Warn("failed to parse NAV", zap.String("nav", d.NAV), zap.Error(err))
			continue
		}

		navHistory = append(navHistory, models.NAVHistory{
			FundID: fundID,
			Date:   date,
			NAV:    nav,
		})
	}

	return navHistory, nil
}

func (c *MFAPIClient) GetRateLimitStatus() RateLimitStatus {
	return c.rateLimiter.GetStatus()
}
