package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/config"
)

type SQLiteDB struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewSQLiteDB(ctx context.Context, cfg config.DatabaseConfig, logger *zap.Logger) (*SQLiteDB, error) {
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", cfg.Path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=2000&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	sqliteDB := &SQLiteDB{
		db:     db,
		logger: logger,
	}

	if err := sqliteDB.migrate(ctx); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return sqliteDB, nil
}

func (s *SQLiteDB) DB() *sql.DB {
	return s.db
}

func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

func (s *SQLiteDB) migrate(ctx context.Context) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS amcs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS analytics_windows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			days INTEGER NOT NULL,
			label TEXT NOT NULL,
			is_active INTEGER DEFAULT 1
		)`,

		`CREATE TABLE IF NOT EXISTS funds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scheme_code INTEGER NOT NULL UNIQUE,
			scheme_name TEXT NOT NULL,
			amc_id INTEGER NOT NULL REFERENCES amcs(id),
			category_id INTEGER NOT NULL REFERENCES categories(id),
			isin_growth TEXT,
			is_active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE INDEX IF NOT EXISTS idx_funds_scheme_code ON funds(scheme_code)`,
		`CREATE INDEX IF NOT EXISTS idx_funds_amc_id ON funds(amc_id)`,
		`CREATE INDEX IF NOT EXISTS idx_funds_category_id ON funds(category_id)`,

		`CREATE TABLE IF NOT EXISTS nav_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			fund_id INTEGER NOT NULL REFERENCES funds(id),
			date DATE NOT NULL,
			nav REAL NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(fund_id, date)
		)`,

		`CREATE INDEX IF NOT EXISTS idx_nav_history_fund_date ON nav_history(fund_id, date DESC)`,

		`CREATE TABLE IF NOT EXISTS fund_analytics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			fund_id INTEGER NOT NULL REFERENCES funds(id),
			window_id INTEGER NOT NULL REFERENCES analytics_windows(id),
			rolling_return REAL,
			cagr REAL,
			max_drawdown REAL,
			volatility REAL,
			sharpe_ratio REAL,
			calculated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			data_start_date DATE,
			data_end_date DATE,
			nav_data_point_count INTEGER DEFAULT 0,
			UNIQUE(fund_id, window_id)
		)`,

		`CREATE INDEX IF NOT EXISTS idx_fund_analytics_fund_window ON fund_analytics(fund_id, window_id)`,

		`CREATE TABLE IF NOT EXISTS sync_state (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			fund_id INTEGER NOT NULL REFERENCES funds(id),
			sync_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			last_synced_at DATETIME,
			last_nav_date DATE,
			total_records INTEGER DEFAULT 0,
			error_message TEXT,
			retry_count INTEGER DEFAULT 0,
			next_retry_at DATETIME,
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME,
			resume_pointer TEXT,
			UNIQUE(fund_id, sync_type)
		)`,

		`CREATE INDEX IF NOT EXISTS idx_sync_state_fund_type ON sync_state(fund_id, sync_type)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_state_status ON sync_state(status)`,

		`CREATE TABLE IF NOT EXISTS rate_limit_state (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			window_type TEXT NOT NULL,
			window_start DATETIME NOT NULL,
			count INTEGER DEFAULT 0,
			blocked_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(window_type, window_start)
		)`,

		`CREATE INDEX IF NOT EXISTS idx_rate_limit_window ON rate_limit_state(window_type, window_start)`,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, migration := range migrations {
		if _, err := tx.ExecContext(ctx, migration); err != nil {
			return fmt.Errorf("failed to execute migration: %w", err)
		}
	}

	if err := s.seedData(ctx, tx); err != nil {
		return fmt.Errorf("failed to seed data: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteDB) seedData(ctx context.Context, tx *sql.Tx) error {
	windows := []struct {
		code  string
		days  int
		label string
	}{
		{"1Y", 365, "1 Year"},
		{"3Y", 1095, "3 Years"},
		{"5Y", 1825, "5 Years"},
		{"10Y", 3650, "10 Years"},
	}

	for _, w := range windows {
		_, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO analytics_windows (code, days, label)
			VALUES (?, ?, ?)
		`, w.code, w.days, w.label)
		if err != nil {
			return fmt.Errorf("failed to seed analytics window %s: %w", w.code, err)
		}
	}

	return nil
}

func (s *SQLiteDB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			s.logger.Error("failed to rollback transaction", zap.Error(rbErr))
		}
		return err
	}

	return tx.Commit()
}

func (s *SQLiteDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

func (s *SQLiteDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

func (s *SQLiteDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

func (s *SQLiteDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, opts)
}

func parseTime(value interface{}) *time.Time {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		return &v
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05", v)
			if err != nil {
				return nil
			}
		}
		return &t
	}
	return nil
}
