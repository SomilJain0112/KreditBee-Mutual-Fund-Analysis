package services

import (
	"context"
	"testing"
	"time"

	"github.com/kreditbee/mf-analytics/internal/config"
	"github.com/kreditbee/mf-analytics/internal/repository"
	"github.com/kreditbee/mf-analytics/pkg/logger"
)

func setupTestDB(t *testing.T) (*repository.SQLiteDB, func()) {
	t.Helper()

	log := logger.NewNop()
	cfg := config.DatabaseConfig{
		Path:            ":memory:",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	}

	db, err := repository.NewSQLiteDB(context.Background(), cfg, log)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	return db, func() {
		db.Close()
	}
}

func TestRateLimiter_Acquire_WithinLimits(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewRateLimitRepository(db)
	log := logger.NewNop()

	cfg := config.MFAPIConfig{
		RequestsPerSec:  5,
		RequestsPerMin:  100,
		RequestsPerHour: 1000,
		BlockDuration:   5 * time.Minute,
	}

	rl := NewRateLimiter(cfg, repo, log)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := rl.Acquire(ctx)
		if err != nil {
			t.Errorf("unexpected error on acquire %d: %v", i, err)
		}
	}

	status := rl.GetStatus()
	if status.SecondCount < 1 {
		t.Errorf("expected second count >= 1, got %d", status.SecondCount)
	}
}

func TestRateLimiter_Acquire_ExceedsSecondLimit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewRateLimitRepository(db)
	log := logger.NewNop()

	cfg := config.MFAPIConfig{
		RequestsPerSec:  2,
		RequestsPerMin:  100,
		RequestsPerHour: 1000,
		BlockDuration:   5 * time.Minute,
	}

	rl := NewRateLimiter(cfg, repo, log)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		err := rl.Acquire(ctx)
		if err != nil && err != ctx.Err() {
			t.Logf("acquire %d: %v", i, err)
		}
	}

	status := rl.GetStatus()
	if status.SecondLimit != 2 {
		t.Errorf("expected second limit 2, got %d", status.SecondLimit)
	}
}

func TestRateLimiter_ExceedsMinuteLimit_Blocks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewRateLimitRepository(db)
	log := logger.NewNop()

	cfg := config.MFAPIConfig{
		RequestsPerSec:  100,
		RequestsPerMin:  3,
		RequestsPerHour: 1000,
		BlockDuration:   100 * time.Millisecond,
	}

	rl := NewRateLimiter(cfg, repo, log)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = rl.Acquire(ctx)
	}

	err := rl.Acquire(ctx)
	if err != ErrRateLimitExceeded {
		t.Errorf("expected ErrRateLimitExceeded, got %v", err)
	}

	status := rl.GetStatus()
	if !status.IsBlocked {
		t.Error("expected rate limiter to be blocked")
	}
}

func TestRateLimiter_GetStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewRateLimitRepository(db)
	log := logger.NewNop()

	cfg := config.MFAPIConfig{
		RequestsPerSec:  2,
		RequestsPerMin:  50,
		RequestsPerHour: 300,
		BlockDuration:   5 * time.Minute,
	}

	rl := NewRateLimiter(cfg, repo, log)
	status := rl.GetStatus()

	if status.SecondLimit != 2 {
		t.Errorf("expected second limit 2, got %d", status.SecondLimit)
	}
	if status.MinuteLimit != 50 {
		t.Errorf("expected minute limit 50, got %d", status.MinuteLimit)
	}
	if status.HourLimit != 300 {
		t.Errorf("expected hour limit 300, got %d", status.HourLimit)
	}
	if status.IsBlocked {
		t.Error("expected not blocked initially")
	}
}

func TestRateLimiter_WindowReset(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewRateLimitRepository(db)
	log := logger.NewNop()

	cfg := config.MFAPIConfig{
		RequestsPerSec:  2,
		RequestsPerMin:  100,
		RequestsPerHour: 1000,
		BlockDuration:   5 * time.Minute,
	}

	rl := NewRateLimiter(cfg, repo, log)

	ctx := context.Background()
	_ = rl.Acquire(ctx)
	_ = rl.Acquire(ctx)

	time.Sleep(1100 * time.Millisecond)

	err := rl.Acquire(ctx)
	if err != nil {
		t.Errorf("expected no error after window reset, got %v", err)
	}
}

func TestRateLimiter_Persistence(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := repository.NewRateLimitRepository(db)
	log := logger.NewNop()

	cfg := config.MFAPIConfig{
		RequestsPerSec:  10,
		RequestsPerMin:  100,
		RequestsPerHour: 1000,
		BlockDuration:   5 * time.Minute,
	}

	rl := NewRateLimiter(cfg, repo, log)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = rl.Acquire(ctx)
	}

	time.Sleep(100 * time.Millisecond)

	counts, err := repo.GetAllCurrentCounts(ctx)
	if err != nil {
		t.Fatalf("failed to get counts: %v", err)
	}

	if counts["minute"] < 1 {
		t.Errorf("expected minute count >= 1, got %d", counts["minute"])
	}
}
