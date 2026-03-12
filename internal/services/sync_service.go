package services

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/config"
	"github.com/kreditbee/mf-analytics/internal/models"
	"github.com/kreditbee/mf-analytics/internal/repository"
)

type SyncService struct {
	cfg        config.SyncConfig
	fundRepo   *repository.FundRepository
	syncRepo   *repository.SyncRepository
	mfClient   *MFAPIClient
	logger     *zap.Logger
	mu         sync.Mutex
	isRunning  bool
	cancelFunc context.CancelFunc
}

func NewSyncService(
	cfg config.SyncConfig,
	fundRepo *repository.FundRepository,
	syncRepo *repository.SyncRepository,
	mfClient *MFAPIClient,
	logger *zap.Logger,
) *SyncService {
	return &SyncService{
		cfg:      cfg,
		fundRepo: fundRepo,
		syncRepo: syncRepo,
		mfClient: mfClient,
		logger:   logger,
	}
}

func (s *SyncService) TriggerSync(ctx context.Context, syncType string) error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("sync already in progress")
	}
	s.isRunning = true
	syncCtx, cancel := context.WithCancel(context.Background())
	s.cancelFunc = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.isRunning = false
			s.cancelFunc = nil
			s.mu.Unlock()
		}()

		if err := s.runSync(syncCtx, syncType); err != nil {
			s.logger.Error("sync failed", zap.String("type", syncType), zap.Error(err))
		}
	}()

	return nil
}

func (s *SyncService) runSync(ctx context.Context, syncType string) error {
	funds, err := s.fundRepo.ListFunds(ctx)
	if err != nil {
		return fmt.Errorf("failed to list funds: %w", err)
	}

	s.logger.Info("starting sync", zap.String("type", syncType), zap.Int("fund_count", len(funds)))

	for _, fund := range funds {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := s.syncFund(ctx, fund.ID, fund.SchemeCode, syncType); err != nil {
			s.logger.Error("failed to sync fund",
				zap.Int64("fund_id", fund.ID),
				zap.Int64("scheme_code", fund.SchemeCode),
				zap.Error(err),
			)
			continue
		}
	}

	return nil
}

func (s *SyncService) syncFund(ctx context.Context, fundID, schemeCode int64, syncType string) error {
	state, err := s.syncRepo.GetSyncState(ctx, fundID, syncType)
	if err != nil {
		return fmt.Errorf("failed to get sync state: %w", err)
	}

	if state == nil {
		state = &models.SyncState{
			FundID:    fundID,
			SyncType:  syncType,
			Status:    models.SyncStatusPending,
			StartedAt: time.Now(),
		}
		if err := s.syncRepo.UpsertSyncState(ctx, state); err != nil {
			return fmt.Errorf("failed to create sync state: %w", err)
		}
	}

	if state.Status == models.SyncStatusInProgress {
		s.logger.Info("sync already in progress, skipping", zap.Int64("fund_id", fundID))
		return nil
	}

	if err := s.syncRepo.MarkSyncInProgress(ctx, fundID, syncType); err != nil {
		return fmt.Errorf("failed to mark sync in progress: %w", err)
	}

	apiResp, err := s.mfClient.FetchSchemeData(ctx, schemeCode)
	if err != nil {
		nextRetry := time.Now().Add(s.cfg.RetryDelay * time.Duration(state.RetryCount+1))
		if err := s.syncRepo.MarkSyncFailed(ctx, fundID, syncType, err.Error(), &nextRetry); err != nil {
			s.logger.Error("failed to mark sync failed", zap.Error(err))
		}
		return fmt.Errorf("failed to fetch scheme data: %w", err)
	}

	navHistory, err := s.mfClient.ParseNAVData(fundID, apiResp.Data)
	if err != nil {
		return fmt.Errorf("failed to parse NAV data: %w", err)
	}

	if syncType == models.SyncTypeIncremental && state.LastNAVDate != nil {
		var filteredNav []models.NAVHistory
		for _, nav := range navHistory {
			if nav.Date.After(*state.LastNAVDate) {
				filteredNav = append(filteredNav, nav)
			}
		}
		navHistory = filteredNav
		s.logger.Debug("filtered NAV for incremental sync",
			zap.Int64("fund_id", fundID),
			zap.Int("filtered_count", len(navHistory)),
		)
	}

	if len(navHistory) == 0 {
		s.logger.Debug("no new NAV data", zap.Int64("fund_id", fundID))
		if err := s.syncRepo.MarkSyncCompleted(ctx, fundID, syncType, state.TotalRecords, state.LastNAVDate); err != nil {
			return fmt.Errorf("failed to mark sync completed: %w", err)
		}
		return nil
	}

	inserted, err := s.insertNAVWithResumability(ctx, fundID, syncType, navHistory)
	if err != nil {
		return fmt.Errorf("failed to insert NAV data: %w", err)
	}

	sort.Slice(navHistory, func(i, j int) bool {
		return navHistory[i].Date.After(navHistory[j].Date)
	})
	latestDate := navHistory[0].Date

	totalRecords, err := s.fundRepo.GetNAVCount(ctx, fundID)
	if err != nil {
		s.logger.Warn("failed to get NAV count", zap.Error(err))
		totalRecords = int(inserted)
	}

	if err := s.syncRepo.MarkSyncCompleted(ctx, fundID, syncType, totalRecords, &latestDate); err != nil {
		return fmt.Errorf("failed to mark sync completed: %w", err)
	}

	s.logger.Info("sync completed",
		zap.Int64("fund_id", fundID),
		zap.Int64("inserted", inserted),
		zap.Int("total_records", totalRecords),
	)

	return nil
}

func (s *SyncService) insertNAVWithResumability(ctx context.Context, fundID int64, syncType string, navHistory []models.NAVHistory) (int64, error) {
	batchSize := s.cfg.BackfillBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	var totalInserted int64
	for i := 0; i < len(navHistory); i += batchSize {
		select {
		case <-ctx.Done():
			pointer := fmt.Sprintf("%d", i)
			if err := s.syncRepo.UpdateResumePointer(ctx, fundID, syncType, pointer); err != nil {
				s.logger.Error("failed to update resume pointer", zap.Error(err))
			}
			return totalInserted, ctx.Err()
		default:
		}

		end := i + batchSize
		if end > len(navHistory) {
			end = len(navHistory)
		}

		batch := navHistory[i:end]
		inserted, err := s.fundRepo.InsertNAVBatch(ctx, batch)
		if err != nil {
			pointer := fmt.Sprintf("%d", i)
			if err := s.syncRepo.UpdateResumePointer(ctx, fundID, syncType, pointer); err != nil {
				s.logger.Error("failed to update resume pointer", zap.Error(err))
			}
			return totalInserted, fmt.Errorf("failed to insert batch at index %d: %w", i, err)
		}
		totalInserted += inserted
	}

	return totalInserted, nil
}

func (s *SyncService) ResumePausedSyncs(ctx context.Context) error {
	states, err := s.syncRepo.GetPendingOrFailedSyncs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending syncs: %w", err)
	}

	for _, state := range states {
		if state.Status == models.SyncStatusPaused && state.ResumePointer != "" {
			s.logger.Info("resuming paused sync",
				zap.Int64("fund_id", state.FundID),
				zap.String("pointer", state.ResumePointer),
			)
		}

		fund, err := s.fundRepo.GetFundByID(ctx, state.FundID)
		if err != nil || fund == nil {
			continue
		}

		if err := s.syncFund(ctx, fund.ID, fund.SchemeCode, state.SyncType); err != nil {
			s.logger.Error("failed to resume sync", zap.Error(err))
		}
	}

	return nil
}

func (s *SyncService) GetSyncStatus(ctx context.Context) (*SyncStatusResponse, error) {
	states, err := s.syncRepo.GetAllSyncStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sync states: %w", err)
	}

	funds, err := s.fundRepo.ListFunds(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list funds: %w", err)
	}

	fundMap := make(map[int64]models.FundWithDetails)
	for _, f := range funds {
		fundMap[f.ID] = f
	}

	var fundStatuses []FundSyncStatus
	for _, state := range states {
		fund, ok := fundMap[state.FundID]
		if !ok {
			continue
		}

		fundStatuses = append(fundStatuses, FundSyncStatus{
			SchemeCode:   fund.SchemeCode,
			SchemeName:   fund.SchemeName,
			SyncType:     state.SyncType,
			Status:       state.Status,
			LastSyncedAt: state.LastSyncedAt,
			LastNAVDate:  state.LastNAVDate,
			TotalRecords: state.TotalRecords,
			ErrorMessage: state.ErrorMessage,
			RetryCount:   state.RetryCount,
		})
	}

	s.mu.Lock()
	isRunning := s.isRunning
	s.mu.Unlock()

	return &SyncStatusResponse{
		IsRunning:       isRunning,
		RateLimitStatus: s.mfClient.GetRateLimitStatus(),
		Funds:           fundStatuses,
	}, nil
}

func (s *SyncService) StopSync() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancelFunc != nil {
		s.cancelFunc()
	}
}

func (s *SyncService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}

func (s *SyncService) GetResumePointer(ctx context.Context, fundID int64, syncType string) (int, error) {
	state, err := s.syncRepo.GetSyncState(ctx, fundID, syncType)
	if err != nil || state == nil {
		return 0, err
	}
	if state.ResumePointer == "" {
		return 0, nil
	}
	parts := strings.Split(state.ResumePointer, ":")
	if len(parts) > 0 {
		idx, _ := strconv.Atoi(parts[0])
		return idx, nil
	}
	return 0, nil
}

type SyncStatusResponse struct {
	IsRunning       bool            `json:"is_running"`
	RateLimitStatus RateLimitStatus `json:"rate_limit_status"`
	Funds           []FundSyncStatus `json:"funds"`
}

type FundSyncStatus struct {
	SchemeCode   int64      `json:"scheme_code"`
	SchemeName   string     `json:"scheme_name"`
	SyncType     string     `json:"sync_type"`
	Status       string     `json:"status"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	LastNAVDate  *time.Time `json:"last_nav_date,omitempty"`
	TotalRecords int        `json:"total_records"`
	ErrorMessage string     `json:"error_message,omitempty"`
	RetryCount   int        `json:"retry_count"`
}
