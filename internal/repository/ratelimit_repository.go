package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kreditbee/mf-analytics/internal/models"
)

type RateLimitRepository struct {
	db *SQLiteDB
}

func NewRateLimitRepository(db *SQLiteDB) *RateLimitRepository {
	return &RateLimitRepository{db: db}
}

func (r *RateLimitRepository) GetOrCreateWindow(ctx context.Context, windowType string, windowStart time.Time) (*models.RateLimitState, error) {
	var state models.RateLimitState
	var blockedAt sql.NullTime

	windowStartStr := windowStart.Format(time.RFC3339)
	err := r.db.QueryRowContext(ctx, `
		SELECT id, window_type, window_start, count, blocked_at, created_at, updated_at
		FROM rate_limit_state
		WHERE window_type = ? AND window_start = ?
	`, windowType, windowStartStr).Scan(
		&state.ID, &state.WindowType, &state.WindowStart, &state.Count,
		&blockedAt, &state.CreatedAt, &state.UpdatedAt,
	)
	if err == nil {
		if blockedAt.Valid {
			state.BlockedAt = blockedAt.Time
		}
		return &state, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query rate limit state: %w", err)
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO rate_limit_state (window_type, window_start, count)
		VALUES (?, ?, 0)
	`, windowType, windowStartStr)
	if err != nil {
		return nil, fmt.Errorf("failed to insert rate limit state: %w", err)
	}

	id, _ := result.LastInsertId()
	return &models.RateLimitState{
		ID:          id,
		WindowType:  windowType,
		WindowStart: windowStart,
		Count:       0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

func (r *RateLimitRepository) IncrementCount(ctx context.Context, windowType string, windowStart time.Time) (int, error) {
	windowStartStr := windowStart.Format(time.RFC3339)
	result, err := r.db.ExecContext(ctx, `
		UPDATE rate_limit_state
		SET count = count + 1, updated_at = CURRENT_TIMESTAMP
		WHERE window_type = ? AND window_start = ?
	`, windowType, windowStartStr)
	if err != nil {
		return 0, fmt.Errorf("failed to increment count: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		_, err = r.db.ExecContext(ctx, `
			INSERT INTO rate_limit_state (window_type, window_start, count)
			VALUES (?, ?, 1)
		`, windowType, windowStartStr)
		if err != nil {
			return 0, fmt.Errorf("failed to insert rate limit state: %w", err)
		}
		return 1, nil
	}

	var count int
	err = r.db.QueryRowContext(ctx, `
		SELECT count FROM rate_limit_state
		WHERE window_type = ? AND window_start = ?
	`, windowType, windowStartStr).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get count: %w", err)
	}
	return count, nil
}

func (r *RateLimitRepository) GetCurrentCount(ctx context.Context, windowType string, windowStart time.Time) (int, error) {
	var count int
	windowStartStr := windowStart.Format(time.RFC3339)
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(count, 0) FROM rate_limit_state
		WHERE window_type = ? AND window_start = ?
	`, windowType, windowStartStr).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get count: %w", err)
	}
	return count, nil
}

func (r *RateLimitRepository) SetBlocked(ctx context.Context, windowType string, blockedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE rate_limit_state
		SET blocked_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE window_type = ? AND window_start = (
			SELECT window_start FROM rate_limit_state 
			WHERE window_type = ? 
			ORDER BY window_start DESC 
			LIMIT 1
		)
	`, blockedAt, windowType, windowType)
	return err
}

func (r *RateLimitRepository) GetBlockedUntil(ctx context.Context, windowType string) (*time.Time, error) {
	var blockedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT blocked_at FROM rate_limit_state
		WHERE window_type = ? AND blocked_at IS NOT NULL
		ORDER BY blocked_at DESC
		LIMIT 1
	`, windowType).Scan(&blockedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blocked until: %w", err)
	}
	if blockedAt.Valid {
		return &blockedAt.Time, nil
	}
	return nil, nil
}

func (r *RateLimitRepository) CleanupOldRecords(ctx context.Context, olderThan time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM rate_limit_state
		WHERE window_start < ?
	`, olderThan.Format(time.RFC3339))
	return err
}

func (r *RateLimitRepository) GetAllCurrentCounts(ctx context.Context) (map[string]int, error) {
	now := time.Now()
	secondStart := now.Truncate(time.Second)
	minuteStart := now.Truncate(time.Minute)
	hourStart := now.Truncate(time.Hour)

	counts := make(map[string]int)

	secCount, err := r.GetCurrentCount(ctx, models.RateLimitWindowSecond, secondStart)
	if err != nil {
		return nil, err
	}
	counts[models.RateLimitWindowSecond] = secCount

	minCount, err := r.GetCurrentCount(ctx, models.RateLimitWindowMinute, minuteStart)
	if err != nil {
		return nil, err
	}
	counts[models.RateLimitWindowMinute] = minCount

	hourCount, err := r.GetCurrentCount(ctx, models.RateLimitWindowHour, hourStart)
	if err != nil {
		return nil, err
	}
	counts[models.RateLimitWindowHour] = hourCount

	return counts, nil
}
