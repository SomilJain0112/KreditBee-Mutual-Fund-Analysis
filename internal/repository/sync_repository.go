package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kreditbee/mf-analytics/internal/models"
)

type SyncRepository struct {
	db *SQLiteDB
}

func NewSyncRepository(db *SQLiteDB) *SyncRepository {
	return &SyncRepository{db: db}
}

func (r *SyncRepository) GetSyncState(ctx context.Context, fundID int64, syncType string) (*models.SyncState, error) {
	var state models.SyncState
	var lastSyncedAt, lastNAVDate, nextRetryAt, completedAt sql.NullTime
	var errorMsg, resumePointer sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, fund_id, sync_type, status, last_synced_at, last_nav_date, total_records,
		       error_message, retry_count, next_retry_at, started_at, completed_at, resume_pointer
		FROM sync_state
		WHERE fund_id = ? AND sync_type = ?
	`, fundID, syncType).Scan(
		&state.ID, &state.FundID, &state.SyncType, &state.Status,
		&lastSyncedAt, &lastNAVDate, &state.TotalRecords,
		&errorMsg, &state.RetryCount, &nextRetryAt,
		&state.StartedAt, &completedAt, &resumePointer,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get sync state: %w", err)
	}

	if lastSyncedAt.Valid {
		state.LastSyncedAt = &lastSyncedAt.Time
	}
	if lastNAVDate.Valid {
		state.LastNAVDate = &lastNAVDate.Time
	}
	if nextRetryAt.Valid {
		state.NextRetryAt = &nextRetryAt.Time
	}
	if completedAt.Valid {
		state.CompletedAt = &completedAt.Time
	}
	state.ErrorMessage = errorMsg.String
	state.ResumePointer = resumePointer.String

	return &state, nil
}

func (r *SyncRepository) UpsertSyncState(ctx context.Context, state *models.SyncState) error {
	var lastNAVDate, nextRetryAt, completedAt interface{}
	if state.LastNAVDate != nil {
		lastNAVDate = state.LastNAVDate.Format("2006-01-02")
	}
	if state.NextRetryAt != nil {
		nextRetryAt = *state.NextRetryAt
	}
	if state.CompletedAt != nil {
		completedAt = *state.CompletedAt
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sync_state (fund_id, sync_type, status, last_synced_at, last_nav_date, 
		                        total_records, error_message, retry_count, next_retry_at,
		                        started_at, completed_at, resume_pointer)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(fund_id, sync_type) DO UPDATE SET
			status = excluded.status,
			last_synced_at = excluded.last_synced_at,
			last_nav_date = excluded.last_nav_date,
			total_records = excluded.total_records,
			error_message = excluded.error_message,
			retry_count = excluded.retry_count,
			next_retry_at = excluded.next_retry_at,
			completed_at = excluded.completed_at,
			resume_pointer = excluded.resume_pointer
	`, state.FundID, state.SyncType, state.Status, state.LastSyncedAt, lastNAVDate,
		state.TotalRecords, state.ErrorMessage, state.RetryCount, nextRetryAt,
		state.StartedAt, completedAt, state.ResumePointer)
	return err
}

func (r *SyncRepository) GetAllSyncStates(ctx context.Context) ([]models.SyncState, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, fund_id, sync_type, status, last_synced_at, last_nav_date, total_records,
		       error_message, retry_count, next_retry_at, started_at, completed_at, resume_pointer
		FROM sync_state
		ORDER BY fund_id, sync_type
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get sync states: %w", err)
	}
	defer rows.Close()

	var states []models.SyncState
	for rows.Next() {
		var state models.SyncState
		var lastSyncedAt, lastNAVDate, nextRetryAt, completedAt sql.NullTime
		var errorMsg, resumePointer sql.NullString

		if err := rows.Scan(
			&state.ID, &state.FundID, &state.SyncType, &state.Status,
			&lastSyncedAt, &lastNAVDate, &state.TotalRecords,
			&errorMsg, &state.RetryCount, &nextRetryAt,
			&state.StartedAt, &completedAt, &resumePointer,
		); err != nil {
			return nil, fmt.Errorf("failed to scan sync state: %w", err)
		}

		if lastSyncedAt.Valid {
			state.LastSyncedAt = &lastSyncedAt.Time
		}
		if lastNAVDate.Valid {
			state.LastNAVDate = &lastNAVDate.Time
		}
		if nextRetryAt.Valid {
			state.NextRetryAt = &nextRetryAt.Time
		}
		if completedAt.Valid {
			state.CompletedAt = &completedAt.Time
		}
		state.ErrorMessage = errorMsg.String
		state.ResumePointer = resumePointer.String

		states = append(states, state)
	}
	return states, rows.Err()
}

func (r *SyncRepository) GetPendingOrFailedSyncs(ctx context.Context) ([]models.SyncState, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, fund_id, sync_type, status, last_synced_at, last_nav_date, total_records,
		       error_message, retry_count, next_retry_at, started_at, completed_at, resume_pointer
		FROM sync_state
		WHERE status IN (?, ?, ?) AND (next_retry_at IS NULL OR next_retry_at <= ?)
		ORDER BY started_at ASC
	`, models.SyncStatusPending, models.SyncStatusFailed, models.SyncStatusPaused, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to get pending syncs: %w", err)
	}
	defer rows.Close()

	var states []models.SyncState
	for rows.Next() {
		var state models.SyncState
		var lastSyncedAt, lastNAVDate, nextRetryAt, completedAt sql.NullTime
		var errorMsg, resumePointer sql.NullString

		if err := rows.Scan(
			&state.ID, &state.FundID, &state.SyncType, &state.Status,
			&lastSyncedAt, &lastNAVDate, &state.TotalRecords,
			&errorMsg, &state.RetryCount, &nextRetryAt,
			&state.StartedAt, &completedAt, &resumePointer,
		); err != nil {
			return nil, fmt.Errorf("failed to scan sync state: %w", err)
		}

		if lastSyncedAt.Valid {
			state.LastSyncedAt = &lastSyncedAt.Time
		}
		if lastNAVDate.Valid {
			state.LastNAVDate = &lastNAVDate.Time
		}
		if nextRetryAt.Valid {
			state.NextRetryAt = &nextRetryAt.Time
		}
		if completedAt.Valid {
			state.CompletedAt = &completedAt.Time
		}
		state.ErrorMessage = errorMsg.String
		state.ResumePointer = resumePointer.String

		states = append(states, state)
	}
	return states, rows.Err()
}

func (r *SyncRepository) MarkSyncInProgress(ctx context.Context, fundID int64, syncType string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE sync_state
		SET status = ?, started_at = ?
		WHERE fund_id = ? AND sync_type = ?
	`, models.SyncStatusInProgress, time.Now(), fundID, syncType)
	return err
}

func (r *SyncRepository) MarkSyncCompleted(ctx context.Context, fundID int64, syncType string, totalRecords int, lastNAVDate *time.Time) error {
	now := time.Now()
	var lastNAV interface{}
	if lastNAVDate != nil {
		lastNAV = lastNAVDate.Format("2006-01-02")
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE sync_state
		SET status = ?, completed_at = ?, last_synced_at = ?, total_records = ?, 
		    last_nav_date = ?, error_message = '', retry_count = 0, resume_pointer = ''
		WHERE fund_id = ? AND sync_type = ?
	`, models.SyncStatusCompleted, now, now, totalRecords, lastNAV, fundID, syncType)
	return err
}

func (r *SyncRepository) MarkSyncFailed(ctx context.Context, fundID int64, syncType string, errMsg string, nextRetry *time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE sync_state
		SET status = ?, error_message = ?, retry_count = retry_count + 1, next_retry_at = ?
		WHERE fund_id = ? AND sync_type = ?
	`, models.SyncStatusFailed, errMsg, nextRetry, fundID, syncType)
	return err
}

func (r *SyncRepository) UpdateResumePointer(ctx context.Context, fundID int64, syncType string, pointer string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE sync_state
		SET resume_pointer = ?, status = ?
		WHERE fund_id = ? AND sync_type = ?
	`, pointer, models.SyncStatusPaused, fundID, syncType)
	return err
}
