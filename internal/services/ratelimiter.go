package services

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/config"
	"github.com/kreditbee/mf-analytics/internal/models"
	"github.com/kreditbee/mf-analytics/internal/repository"
)

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	ErrBlocked           = errors.New("requests blocked due to rate limit violation")
)

type RateLimiter struct {
	cfg           config.MFAPIConfig
	repo          *repository.RateLimitRepository
	logger        *zap.Logger
	mu            sync.Mutex
	blockedUntil  time.Time
	lastSecond    time.Time
	lastMinute    time.Time
	lastHour      time.Time
	secondCount   int
	minuteCount   int
	hourCount     int
}

func NewRateLimiter(cfg config.MFAPIConfig, repo *repository.RateLimitRepository, logger *zap.Logger) *RateLimiter {
	rl := &RateLimiter{
		cfg:    cfg,
		repo:   repo,
		logger: logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rl.loadState(ctx)

	return rl
}

func (r *RateLimiter) loadState(ctx context.Context) {
	now := time.Now()

	counts, err := r.repo.GetAllCurrentCounts(ctx)
	if err != nil {
		r.logger.Warn("failed to load rate limit state", zap.Error(err))
		return
	}

	r.lastSecond = now.Truncate(time.Second)
	r.lastMinute = now.Truncate(time.Minute)
	r.lastHour = now.Truncate(time.Hour)
	r.secondCount = counts[models.RateLimitWindowSecond]
	r.minuteCount = counts[models.RateLimitWindowMinute]
	r.hourCount = counts[models.RateLimitWindowHour]

	blocked, err := r.repo.GetBlockedUntil(ctx, "blocked")
	if err != nil {
		r.logger.Warn("failed to check blocked state", zap.Error(err))
		return
	}
	if blocked != nil && blocked.After(now) {
		r.blockedUntil = blocked.Add(r.cfg.BlockDuration)
	}

	r.logger.Info("rate limiter state loaded",
		zap.Int("second_count", r.secondCount),
		zap.Int("minute_count", r.minuteCount),
		zap.Int("hour_count", r.hourCount),
		zap.Time("blocked_until", r.blockedUntil),
	)
}

func (r *RateLimiter) Acquire(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	if now.Before(r.blockedUntil) {
		return ErrBlocked
	}

	r.resetWindowsIfNeeded(now)

	if r.secondCount >= r.cfg.RequestsPerSec {
		r.logger.Debug("rate limit: waiting for second window",
			zap.Int("current", r.secondCount),
			zap.Int("limit", r.cfg.RequestsPerSec),
		)
		waitDuration := r.lastSecond.Add(time.Second).Sub(now)
		if waitDuration > 0 {
			r.mu.Unlock()
			select {
			case <-time.After(waitDuration):
			case <-ctx.Done():
				r.mu.Lock()
				return ctx.Err()
			}
			r.mu.Lock()
			now = time.Now()
			r.resetWindowsIfNeeded(now)
		}
	}

	if r.minuteCount >= r.cfg.RequestsPerMin {
		r.block(ctx, now, "minute limit exceeded")
		return ErrRateLimitExceeded
	}

	if r.hourCount >= r.cfg.RequestsPerHour {
		r.block(ctx, now, "hour limit exceeded")
		return ErrRateLimitExceeded
	}

	r.secondCount++
	r.minuteCount++
	r.hourCount++

	go r.persistCounts(ctx, now)

	return nil
}

func (r *RateLimiter) resetWindowsIfNeeded(now time.Time) {
	currentSecond := now.Truncate(time.Second)
	if currentSecond.After(r.lastSecond) {
		r.secondCount = 0
		r.lastSecond = currentSecond
	}

	currentMinute := now.Truncate(time.Minute)
	if currentMinute.After(r.lastMinute) {
		r.minuteCount = 0
		r.lastMinute = currentMinute
	}

	currentHour := now.Truncate(time.Hour)
	if currentHour.After(r.lastHour) {
		r.hourCount = 0
		r.lastHour = currentHour
	}
}

func (r *RateLimiter) block(ctx context.Context, now time.Time, reason string) {
	r.blockedUntil = now.Add(r.cfg.BlockDuration)
	r.logger.Warn("rate limiter blocked",
		zap.String("reason", reason),
		zap.Time("blocked_until", r.blockedUntil),
	)

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.repo.SetBlocked(bgCtx, "blocked", now); err != nil {
			r.logger.Error("failed to persist blocked state", zap.Error(err))
		}
	}()
}

func (r *RateLimiter) persistCounts(ctx context.Context, now time.Time) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	secondStart := now.Truncate(time.Second)
	if _, err := r.repo.IncrementCount(bgCtx, models.RateLimitWindowSecond, secondStart); err != nil {
		r.logger.Error("failed to persist second count", zap.Error(err))
	}

	minuteStart := now.Truncate(time.Minute)
	if _, err := r.repo.IncrementCount(bgCtx, models.RateLimitWindowMinute, minuteStart); err != nil {
		r.logger.Error("failed to persist minute count", zap.Error(err))
	}

	hourStart := now.Truncate(time.Hour)
	if _, err := r.repo.IncrementCount(bgCtx, models.RateLimitWindowHour, hourStart); err != nil {
		r.logger.Error("failed to persist hour count", zap.Error(err))
	}
}

func (r *RateLimiter) GetStatus() RateLimitStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.resetWindowsIfNeeded(now)

	return RateLimitStatus{
		SecondCount:     r.secondCount,
		SecondLimit:     r.cfg.RequestsPerSec,
		MinuteCount:     r.minuteCount,
		MinuteLimit:     r.cfg.RequestsPerMin,
		HourCount:       r.hourCount,
		HourLimit:       r.cfg.RequestsPerHour,
		IsBlocked:       now.Before(r.blockedUntil),
		BlockedUntil:    r.blockedUntil,
		BlockDuration:   r.cfg.BlockDuration,
	}
}

func (r *RateLimiter) WaitUntilAvailable(ctx context.Context) error {
	r.mu.Lock()
	blockedUntil := r.blockedUntil
	r.mu.Unlock()

	now := time.Now()
	if now.Before(blockedUntil) {
		waitDuration := blockedUntil.Sub(now)
		r.logger.Info("waiting for block to expire", zap.Duration("wait", waitDuration))
		select {
		case <-time.After(waitDuration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return r.Acquire(ctx)
}

func (r *RateLimiter) Cleanup(ctx context.Context) error {
	olderThan := time.Now().Add(-2 * time.Hour)
	return r.repo.CleanupOldRecords(ctx, olderThan)
}

type RateLimitStatus struct {
	SecondCount   int           `json:"second_count"`
	SecondLimit   int           `json:"second_limit"`
	MinuteCount   int           `json:"minute_count"`
	MinuteLimit   int           `json:"minute_limit"`
	HourCount     int           `json:"hour_count"`
	HourLimit     int           `json:"hour_limit"`
	IsBlocked     bool          `json:"is_blocked"`
	BlockedUntil  time.Time     `json:"blocked_until,omitempty"`
	BlockDuration time.Duration `json:"block_duration"`
}
