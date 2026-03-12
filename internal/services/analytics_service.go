package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/models"
	"github.com/kreditbee/mf-analytics/internal/repository"
)

const riskFreeRate = 0.06

type AnalyticsService struct {
	fundRepo *repository.FundRepository
	logger   *zap.Logger
}

func NewAnalyticsService(fundRepo *repository.FundRepository, logger *zap.Logger) *AnalyticsService {
	return &AnalyticsService{
		fundRepo: fundRepo,
		logger:   logger,
	}
}

func (s *AnalyticsService) CalculateAndStoreAnalytics(ctx context.Context, fundID int64) error {
	windows, err := s.fundRepo.GetAnalyticsWindows(ctx)
	if err != nil {
		return fmt.Errorf("failed to get analytics windows: %w", err)
	}

	now := time.Now()

	for _, window := range windows {
		startDate := now.AddDate(0, 0, -window.Days)
		endDate := now

		navHistory, err := s.fundRepo.GetNAVHistory(ctx, fundID, startDate, endDate)
		if err != nil {
			s.logger.Error("failed to get NAV history",
				zap.Int64("fund_id", fundID),
				zap.String("window", window.Code),
				zap.Error(err),
			)
			continue
		}

		if len(navHistory) < 2 {
			s.logger.Debug("insufficient NAV data for analytics",
				zap.Int64("fund_id", fundID),
				zap.String("window", window.Code),
				zap.Int("data_points", len(navHistory)),
			)
			continue
		}

		analytics := s.calculateMetrics(fundID, window.ID, navHistory)
		if analytics == nil {
			continue
		}

		if err := s.fundRepo.UpsertFundAnalytics(ctx, analytics); err != nil {
			s.logger.Error("failed to store analytics",
				zap.Int64("fund_id", fundID),
				zap.String("window", window.Code),
				zap.Error(err),
			)
			continue
		}

		s.logger.Debug("analytics calculated and stored",
			zap.Int64("fund_id", fundID),
			zap.String("window", window.Code),
			zap.String("cagr", analytics.CAGR.StringFixed(4)),
			zap.String("max_drawdown", analytics.MaxDrawdown.StringFixed(4)),
		)
	}

	return nil
}

func (s *AnalyticsService) calculateMetrics(fundID, windowID int64, navHistory []models.NAVHistory) *models.FundAnalytics {
	if len(navHistory) < 2 {
		return nil
	}

	sort.Slice(navHistory, func(i, j int) bool {
		return navHistory[i].Date.Before(navHistory[j].Date)
	})

	startNAV := navHistory[0]
	endNAV := navHistory[len(navHistory)-1]

	startValue := startNAV.NAV.InexactFloat64()
	endValue := endNAV.NAV.InexactFloat64()

	if startValue <= 0 {
		return nil
	}

	totalReturn := (endValue - startValue) / startValue

	years := endNAV.Date.Sub(startNAV.Date).Hours() / (24 * 365)
	if years <= 0 {
		years = 1
	}

	cagr := math.Pow(endValue/startValue, 1/years) - 1

	maxDrawdown := s.calculateMaxDrawdown(navHistory)

	volatility := s.calculateVolatility(navHistory)

	sharpeRatio := decimal.Zero
	if volatility.GreaterThan(decimal.Zero) {
		excessReturn := cagr - riskFreeRate
		sharpeRatio = decimal.NewFromFloat(excessReturn).Div(volatility)
	}

	return &models.FundAnalytics{
		FundID:            fundID,
		WindowID:          windowID,
		RollingReturn:     decimal.NewFromFloat(totalReturn),
		CAGR:              decimal.NewFromFloat(cagr),
		MaxDrawdown:       maxDrawdown,
		Volatility:        volatility,
		SharpeRatio:       sharpeRatio,
		CalculatedAt:      time.Now(),
		DataStartDate:     startNAV.Date,
		DataEndDate:       endNAV.Date,
		NAVDataPointCount: len(navHistory),
	}
}

func (s *AnalyticsService) calculateMaxDrawdown(navHistory []models.NAVHistory) decimal.Decimal {
	if len(navHistory) < 2 {
		return decimal.Zero
	}

	peak := navHistory[0].NAV.InexactFloat64()
	maxDrawdown := 0.0

	for _, nav := range navHistory {
		value := nav.NAV.InexactFloat64()
		if value > peak {
			peak = value
		}
		drawdown := (peak - value) / peak
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}

	return decimal.NewFromFloat(maxDrawdown)
}

func (s *AnalyticsService) calculateVolatility(navHistory []models.NAVHistory) decimal.Decimal {
	if len(navHistory) < 2 {
		return decimal.Zero
	}

	var dailyReturns []float64
	for i := 1; i < len(navHistory); i++ {
		prevNAV := navHistory[i-1].NAV.InexactFloat64()
		currNAV := navHistory[i].NAV.InexactFloat64()
		if prevNAV > 0 {
			dailyReturn := (currNAV - prevNAV) / prevNAV
			dailyReturns = append(dailyReturns, dailyReturn)
		}
	}

	if len(dailyReturns) < 2 {
		return decimal.Zero
	}

	var sum float64
	for _, r := range dailyReturns {
		sum += r
	}
	mean := sum / float64(len(dailyReturns))

	var sumSquaredDiff float64
	for _, r := range dailyReturns {
		diff := r - mean
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(dailyReturns)-1)
	dailyVolatility := math.Sqrt(variance)

	annualizedVolatility := dailyVolatility * math.Sqrt(252)

	return decimal.NewFromFloat(annualizedVolatility)
}

func (s *AnalyticsService) GetFundAnalytics(ctx context.Context, fundID int64) ([]models.FundAnalyticsWithWindow, error) {
	return s.fundRepo.GetFundAnalytics(ctx, fundID)
}

func (s *AnalyticsService) RankFunds(ctx context.Context, windowCode string, sortBy string, limit int) ([]RankedFund, error) {
	results, err := s.fundRepo.GetAllFundsAnalytics(ctx, windowCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get all funds analytics: %w", err)
	}

	var rankedFunds []RankedFund
	for _, r := range results {
		if r.Analytics.ID == 0 {
			continue
		}
		rankedFunds = append(rankedFunds, RankedFund{
			Fund:      r.Fund,
			Analytics: r.Analytics,
		})
	}

	switch sortBy {
	case "cagr":
		sort.Slice(rankedFunds, func(i, j int) bool {
			return rankedFunds[i].Analytics.CAGR.GreaterThan(rankedFunds[j].Analytics.CAGR)
		})
	case "rolling_return":
		sort.Slice(rankedFunds, func(i, j int) bool {
			return rankedFunds[i].Analytics.RollingReturn.GreaterThan(rankedFunds[j].Analytics.RollingReturn)
		})
	case "max_drawdown":
		sort.Slice(rankedFunds, func(i, j int) bool {
			return rankedFunds[i].Analytics.MaxDrawdown.LessThan(rankedFunds[j].Analytics.MaxDrawdown)
		})
	case "sharpe_ratio":
		sort.Slice(rankedFunds, func(i, j int) bool {
			return rankedFunds[i].Analytics.SharpeRatio.GreaterThan(rankedFunds[j].Analytics.SharpeRatio)
		})
	case "volatility":
		sort.Slice(rankedFunds, func(i, j int) bool {
			return rankedFunds[i].Analytics.Volatility.LessThan(rankedFunds[j].Analytics.Volatility)
		})
	default:
		sort.Slice(rankedFunds, func(i, j int) bool {
			return rankedFunds[i].Analytics.CAGR.GreaterThan(rankedFunds[j].Analytics.CAGR)
		})
	}

	for i := range rankedFunds {
		rankedFunds[i].Rank = i + 1
	}

	if limit > 0 && limit < len(rankedFunds) {
		rankedFunds = rankedFunds[:limit]
	}

	return rankedFunds, nil
}

func (s *AnalyticsService) RecalculateAllAnalytics(ctx context.Context) error {
	funds, err := s.fundRepo.ListFunds(ctx)
	if err != nil {
		return fmt.Errorf("failed to list funds: %w", err)
	}

	for _, fund := range funds {
		if err := s.CalculateAndStoreAnalytics(ctx, fund.ID); err != nil {
			s.logger.Error("failed to calculate analytics",
				zap.Int64("fund_id", fund.ID),
				zap.Error(err),
			)
		}
	}

	return nil
}

type RankedFund struct {
	Rank      int                            `json:"rank"`
	Fund      models.FundWithDetails         `json:"fund"`
	Analytics models.FundAnalyticsWithWindow `json:"analytics"`
}
