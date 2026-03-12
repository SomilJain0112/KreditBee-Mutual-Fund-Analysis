package services

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kreditbee/mf-analytics/internal/models"
	"github.com/kreditbee/mf-analytics/internal/repository"
	"github.com/kreditbee/mf-analytics/pkg/logger"
)

func TestAnalyticsService_CalculateMetrics_Basic(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	fundRepo := repository.NewFundRepository(db)
	log := logger.NewNop()
	svc := NewAnalyticsService(fundRepo, log)

	navHistory := []models.NAVHistory{
		{FundID: 1, Date: time.Now().AddDate(-1, 0, 0), NAV: decimal.NewFromFloat(100)},
		{FundID: 1, Date: time.Now().AddDate(0, -6, 0), NAV: decimal.NewFromFloat(110)},
		{FundID: 1, Date: time.Now(), NAV: decimal.NewFromFloat(120)},
	}

	analytics := svc.calculateMetrics(1, 1, navHistory)
	if analytics == nil {
		t.Fatal("expected analytics to be calculated")
	}

	if analytics.RollingReturn.LessThanOrEqual(decimal.Zero) {
		t.Error("expected positive rolling return")
	}

	if analytics.CAGR.LessThanOrEqual(decimal.Zero) {
		t.Error("expected positive CAGR")
	}

	if analytics.NAVDataPointCount != 3 {
		t.Errorf("expected 3 data points, got %d", analytics.NAVDataPointCount)
	}
}

func TestAnalyticsService_CalculateMaxDrawdown(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	fundRepo := repository.NewFundRepository(db)
	log := logger.NewNop()
	svc := NewAnalyticsService(fundRepo, log)

	tests := []struct {
		name        string
		navHistory  []models.NAVHistory
		wantDrawdown float64
	}{
		{
			name: "no drawdown - continuous growth",
			navHistory: []models.NAVHistory{
				{NAV: decimal.NewFromFloat(100)},
				{NAV: decimal.NewFromFloat(110)},
				{NAV: decimal.NewFromFloat(120)},
			},
			wantDrawdown: 0,
		},
		{
			name: "simple drawdown",
			navHistory: []models.NAVHistory{
				{NAV: decimal.NewFromFloat(100)},
				{NAV: decimal.NewFromFloat(80)},
				{NAV: decimal.NewFromFloat(90)},
			},
			wantDrawdown: 0.20,
		},
		{
			name: "peak then drop",
			navHistory: []models.NAVHistory{
				{NAV: decimal.NewFromFloat(100)},
				{NAV: decimal.NewFromFloat(150)},
				{NAV: decimal.NewFromFloat(120)},
			},
			wantDrawdown: 0.20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawdown := svc.calculateMaxDrawdown(tt.navHistory)
			got := drawdown.InexactFloat64()
			if diff := got - tt.wantDrawdown; diff > 0.01 || diff < -0.01 {
				t.Errorf("max drawdown = %v, want %v", got, tt.wantDrawdown)
			}
		})
	}
}

func TestAnalyticsService_CalculateVolatility(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	fundRepo := repository.NewFundRepository(db)
	log := logger.NewNop()
	svc := NewAnalyticsService(fundRepo, log)

	navHistory := []models.NAVHistory{
		{NAV: decimal.NewFromFloat(100)},
		{NAV: decimal.NewFromFloat(102)},
		{NAV: decimal.NewFromFloat(98)},
		{NAV: decimal.NewFromFloat(105)},
		{NAV: decimal.NewFromFloat(101)},
	}

	volatility := svc.calculateVolatility(navHistory)
	if volatility.LessThanOrEqual(decimal.Zero) {
		t.Error("expected positive volatility")
	}
}

func TestAnalyticsService_CalculateVolatility_ZeroForConstantNAV(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	fundRepo := repository.NewFundRepository(db)
	log := logger.NewNop()
	svc := NewAnalyticsService(fundRepo, log)

	navHistory := []models.NAVHistory{
		{NAV: decimal.NewFromFloat(100)},
		{NAV: decimal.NewFromFloat(100)},
		{NAV: decimal.NewFromFloat(100)},
	}

	volatility := svc.calculateVolatility(navHistory)
	if !volatility.Equal(decimal.Zero) {
		t.Errorf("expected zero volatility for constant NAV, got %v", volatility)
	}
}

func TestAnalyticsService_InsufficientData(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	fundRepo := repository.NewFundRepository(db)
	log := logger.NewNop()
	svc := NewAnalyticsService(fundRepo, log)

	navHistory := []models.NAVHistory{
		{FundID: 1, Date: time.Now(), NAV: decimal.NewFromFloat(100)},
	}

	analytics := svc.calculateMetrics(1, 1, navHistory)
	if analytics != nil {
		t.Error("expected nil analytics for insufficient data")
	}
}

func TestAnalyticsService_RankFunds(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	fundRepo := repository.NewFundRepository(db)
	log := logger.NewNop()
	svc := NewAnalyticsService(fundRepo, log)

	amcID, _ := fundRepo.GetOrCreateAMC(ctx, "Test AMC")
	catID, _ := fundRepo.GetOrCreateCategory(ctx, "Test Category")

	fund1 := &models.Fund{
		SchemeCode: 100001,
		SchemeName: "Fund 1",
		AMCID:      amcID,
		CategoryID: catID,
		IsActive:   true,
	}
	fund2 := &models.Fund{
		SchemeCode: 100002,
		SchemeName: "Fund 2",
		AMCID:      amcID,
		CategoryID: catID,
		IsActive:   true,
	}
	_ = fundRepo.UpsertFund(ctx, fund1)
	_ = fundRepo.UpsertFund(ctx, fund2)

	fund1Detail, _ := fundRepo.GetFundBySchemeCode(ctx, 100001)
	fund2Detail, _ := fundRepo.GetFundBySchemeCode(ctx, 100002)

	windows, _ := fundRepo.GetAnalyticsWindows(ctx)
	windowID := windows[0].ID

	analytics1 := &models.FundAnalytics{
		FundID:            fund1Detail.ID,
		WindowID:          windowID,
		CAGR:              decimal.NewFromFloat(0.15),
		RollingReturn:     decimal.NewFromFloat(0.18),
		MaxDrawdown:       decimal.NewFromFloat(0.10),
		Volatility:        decimal.NewFromFloat(0.12),
		SharpeRatio:       decimal.NewFromFloat(0.75),
		CalculatedAt:      time.Now(),
		DataStartDate:     time.Now().AddDate(-1, 0, 0),
		DataEndDate:       time.Now(),
		NAVDataPointCount: 250,
	}
	analytics2 := &models.FundAnalytics{
		FundID:            fund2Detail.ID,
		WindowID:          windowID,
		CAGR:              decimal.NewFromFloat(0.20),
		RollingReturn:     decimal.NewFromFloat(0.22),
		MaxDrawdown:       decimal.NewFromFloat(0.15),
		Volatility:        decimal.NewFromFloat(0.18),
		SharpeRatio:       decimal.NewFromFloat(0.78),
		CalculatedAt:      time.Now(),
		DataStartDate:     time.Now().AddDate(-1, 0, 0),
		DataEndDate:       time.Now(),
		NAVDataPointCount: 250,
	}
	_ = fundRepo.UpsertFundAnalytics(ctx, analytics1)
	_ = fundRepo.UpsertFundAnalytics(ctx, analytics2)

	ranked, err := svc.RankFunds(ctx, "1Y", "cagr", 10)
	if err != nil {
		t.Fatalf("failed to rank funds: %v", err)
	}

	if len(ranked) != 2 {
		t.Errorf("expected 2 ranked funds, got %d", len(ranked))
	}

	if len(ranked) >= 2 {
		if ranked[0].Analytics.CAGR.LessThan(ranked[1].Analytics.CAGR) {
			t.Error("expected funds to be sorted by CAGR descending")
		}
	}
}
