package services

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/models"
	"github.com/kreditbee/mf-analytics/internal/repository"
)

type FundService struct {
	fundRepo         *repository.FundRepository
	syncService      *SyncService
	analyticsService *AnalyticsService
	logger           *zap.Logger
}

func NewFundService(
	fundRepo *repository.FundRepository,
	syncService *SyncService,
	analyticsService *AnalyticsService,
	logger *zap.Logger,
) *FundService {
	return &FundService{
		fundRepo:         fundRepo,
		syncService:      syncService,
		analyticsService: analyticsService,
		logger:           logger,
	}
}

func (s *FundService) InitializeTrackedFunds(ctx context.Context) error {
	for _, tf := range models.TrackedFunds {
		amcID, err := s.fundRepo.GetOrCreateAMC(ctx, tf.AMC)
		if err != nil {
			return fmt.Errorf("failed to create AMC %s: %w", tf.AMC, err)
		}

		categoryID, err := s.fundRepo.GetOrCreateCategory(ctx, tf.Category)
		if err != nil {
			return fmt.Errorf("failed to create category %s: %w", tf.Category, err)
		}

		fund := &models.Fund{
			SchemeCode: tf.SchemeCode,
			SchemeName: fmt.Sprintf("%s %s", tf.AMC, tf.Category),
			AMCID:      amcID,
			CategoryID: categoryID,
			IsActive:   true,
		}

		if err := s.fundRepo.UpsertFund(ctx, fund); err != nil {
			return fmt.Errorf("failed to upsert fund %d: %w", tf.SchemeCode, err)
		}

		s.logger.Debug("initialized fund",
			zap.Int64("scheme_code", tf.SchemeCode),
			zap.String("amc", tf.AMC),
			zap.String("category", tf.Category),
		)
	}

	return nil
}

func (s *FundService) ListFunds(ctx context.Context) ([]models.FundWithDetails, error) {
	return s.fundRepo.ListFunds(ctx)
}

func (s *FundService) GetFund(ctx context.Context, schemeCode int64) (*FundDetailResponse, error) {
	fund, err := s.fundRepo.GetFundBySchemeCode(ctx, schemeCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get fund: %w", err)
	}
	if fund == nil {
		return nil, nil
	}

	latestNAV, err := s.fundRepo.GetLatestNAV(ctx, fund.ID)
	if err != nil {
		s.logger.Warn("failed to get latest NAV", zap.Error(err))
	}

	navCount, err := s.fundRepo.GetNAVCount(ctx, fund.ID)
	if err != nil {
		s.logger.Warn("failed to get NAV count", zap.Error(err))
	}

	return &FundDetailResponse{
		Fund:          *fund,
		LatestNAV:     latestNAV,
		TotalNAVCount: navCount,
	}, nil
}

func (s *FundService) GetFundAnalytics(ctx context.Context, schemeCode int64) (*FundAnalyticsResponse, error) {
	fund, err := s.fundRepo.GetFundBySchemeCode(ctx, schemeCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get fund: %w", err)
	}
	if fund == nil {
		return nil, nil
	}

	analytics, err := s.analyticsService.GetFundAnalytics(ctx, fund.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics: %w", err)
	}

	return &FundAnalyticsResponse{
		Fund:      *fund,
		Analytics: analytics,
	}, nil
}

func (s *FundService) RefreshFundData(ctx context.Context, schemeCode int64) error {
	fund, err := s.fundRepo.GetFundBySchemeCode(ctx, schemeCode)
	if err != nil {
		return fmt.Errorf("failed to get fund: %w", err)
	}
	if fund == nil {
		return fmt.Errorf("fund not found")
	}

	return s.syncService.TriggerSync(ctx, models.SyncTypeIncremental)
}

type FundDetailResponse struct {
	Fund          models.FundWithDetails `json:"fund"`
	LatestNAV     *models.NAVHistory     `json:"latest_nav,omitempty"`
	TotalNAVCount int                    `json:"total_nav_count"`
}

type FundAnalyticsResponse struct {
	Fund      models.FundWithDetails            `json:"fund"`
	Analytics []models.FundAnalyticsWithWindow `json:"analytics"`
}
