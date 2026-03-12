package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type AMC struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Category struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AnalyticsWindow struct {
	ID       int64  `json:"id"`
	Code     string `json:"code"`
	Days     int    `json:"days"`
	Label    string `json:"label"`
	IsActive bool   `json:"is_active"`
}

type Fund struct {
	ID         int64     `json:"id"`
	SchemeCode int64     `json:"scheme_code"`
	SchemeName string    `json:"scheme_name"`
	AMCID      int64     `json:"amc_id"`
	CategoryID int64     `json:"category_id"`
	ISINGrowth string    `json:"isin_growth,omitempty"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type FundWithDetails struct {
	Fund
	AMCName      string `json:"amc_name"`
	CategoryName string `json:"category_name"`
}

type NAVHistory struct {
	ID        int64           `json:"id"`
	FundID    int64           `json:"fund_id"`
	Date      time.Time       `json:"date"`
	NAV       decimal.Decimal `json:"nav"`
	CreatedAt time.Time       `json:"created_at"`
}

type FundAnalytics struct {
	ID                int64           `json:"id"`
	FundID            int64           `json:"fund_id"`
	WindowID          int64           `json:"window_id"`
	RollingReturn     decimal.Decimal `json:"rolling_return"`
	CAGR              decimal.Decimal `json:"cagr"`
	MaxDrawdown       decimal.Decimal `json:"max_drawdown"`
	Volatility        decimal.Decimal `json:"volatility"`
	SharpeRatio       decimal.Decimal `json:"sharpe_ratio"`
	CalculatedAt      time.Time       `json:"calculated_at"`
	DataStartDate     time.Time       `json:"data_start_date"`
	DataEndDate       time.Time       `json:"data_end_date"`
	NAVDataPointCount int             `json:"nav_data_point_count"`
}

type FundAnalyticsWithWindow struct {
	FundAnalytics
	WindowCode  string `json:"window_code"`
	WindowLabel string `json:"window_label"`
}

type SyncState struct {
	ID            int64      `json:"id"`
	FundID        int64      `json:"fund_id"`
	SyncType      string     `json:"sync_type"`
	Status        string     `json:"status"`
	LastSyncedAt  *time.Time `json:"last_synced_at,omitempty"`
	LastNAVDate   *time.Time `json:"last_nav_date,omitempty"`
	TotalRecords  int        `json:"total_records"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	RetryCount    int        `json:"retry_count"`
	NextRetryAt   *time.Time `json:"next_retry_at,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	ResumePointer string     `json:"resume_pointer,omitempty"`
}

type RateLimitState struct {
	ID          int64     `json:"id"`
	WindowType  string    `json:"window_type"`
	WindowStart time.Time `json:"window_start"`
	Count       int       `json:"count"`
	BlockedAt   time.Time `json:"blocked_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const (
	SyncTypeBackfill    = "backfill"
	SyncTypeIncremental = "incremental"

	SyncStatusPending    = "pending"
	SyncStatusInProgress = "in_progress"
	SyncStatusCompleted  = "completed"
	SyncStatusFailed     = "failed"
	SyncStatusPaused     = "paused"

	RateLimitWindowSecond = "second"
	RateLimitWindowMinute = "minute"
	RateLimitWindowHour   = "hour"
)

var TrackedFunds = []struct {
	SchemeCode int64
	AMC        string
	Category   string
}{
	{120505, "ICICI Prudential", "Equity Mid Cap Direct Growth"},
	{120586, "ICICI Prudential", "Equity Small Cap Direct Growth"},
	{118989, "HDFC", "Equity Mid Cap Direct Growth"},
	{130503, "HDFC", "Equity Small Cap Direct Growth"},
	{119028, "Axis", "Equity Mid Cap Direct Growth"},
	{125354, "Axis", "Equity Small Cap Direct Growth"},
	{119551, "SBI", "Equity Mid Cap Direct Growth"},
	{125497, "SBI", "Equity Small Cap Direct Growth"},
	{118668, "Kotak Mahindra", "Equity Mid Cap Direct Growth"},
	{125494, "Kotak Mahindra", "Equity Small Cap Direct Growth"},
}
