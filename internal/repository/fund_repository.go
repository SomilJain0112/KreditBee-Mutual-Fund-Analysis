package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kreditbee/mf-analytics/internal/models"
)

type FundRepository struct {
	db *SQLiteDB
}

func NewFundRepository(db *SQLiteDB) *FundRepository {
	return &FundRepository{db: db}
}

func (r *FundRepository) GetOrCreateAMC(ctx context.Context, name string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, "SELECT id FROM amcs WHERE name = ?", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to query AMC: %w", err)
	}

	result, err := r.db.ExecContext(ctx, "INSERT INTO amcs (name) VALUES (?)", name)
	if err != nil {
		return 0, fmt.Errorf("failed to insert AMC: %w", err)
	}
	return result.LastInsertId()
}

func (r *FundRepository) GetOrCreateCategory(ctx context.Context, name string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, "SELECT id FROM categories WHERE name = ?", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to query category: %w", err)
	}

	result, err := r.db.ExecContext(ctx, "INSERT INTO categories (name) VALUES (?)", name)
	if err != nil {
		return 0, fmt.Errorf("failed to insert category: %w", err)
	}
	return result.LastInsertId()
}

func (r *FundRepository) UpsertFund(ctx context.Context, fund *models.Fund) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO funds (scheme_code, scheme_name, amc_id, category_id, isin_growth, is_active)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(scheme_code) DO UPDATE SET
			scheme_name = excluded.scheme_name,
			amc_id = excluded.amc_id,
			category_id = excluded.category_id,
			isin_growth = excluded.isin_growth,
			is_active = excluded.is_active,
			updated_at = CURRENT_TIMESTAMP
	`, fund.SchemeCode, fund.SchemeName, fund.AMCID, fund.CategoryID, fund.ISINGrowth, fund.IsActive)
	return err
}

func (r *FundRepository) GetFundBySchemeCode(ctx context.Context, schemeCode int64) (*models.FundWithDetails, error) {
	var fund models.FundWithDetails
	var isActive int
	err := r.db.QueryRowContext(ctx, `
		SELECT f.id, f.scheme_code, f.scheme_name, f.amc_id, f.category_id, 
		       COALESCE(f.isin_growth, ''), f.is_active, f.created_at, f.updated_at,
		       a.name, c.name
		FROM funds f
		JOIN amcs a ON f.amc_id = a.id
		JOIN categories c ON f.category_id = c.id
		WHERE f.scheme_code = ?
	`, schemeCode).Scan(
		&fund.ID, &fund.SchemeCode, &fund.SchemeName, &fund.AMCID, &fund.CategoryID,
		&fund.ISINGrowth, &isActive, &fund.CreatedAt, &fund.UpdatedAt,
		&fund.AMCName, &fund.CategoryName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get fund: %w", err)
	}
	fund.IsActive = isActive == 1
	return &fund, nil
}

func (r *FundRepository) GetFundByID(ctx context.Context, id int64) (*models.FundWithDetails, error) {
	var fund models.FundWithDetails
	var isActive int
	err := r.db.QueryRowContext(ctx, `
		SELECT f.id, f.scheme_code, f.scheme_name, f.amc_id, f.category_id, 
		       COALESCE(f.isin_growth, ''), f.is_active, f.created_at, f.updated_at,
		       a.name, c.name
		FROM funds f
		JOIN amcs a ON f.amc_id = a.id
		JOIN categories c ON f.category_id = c.id
		WHERE f.id = ?
	`, id).Scan(
		&fund.ID, &fund.SchemeCode, &fund.SchemeName, &fund.AMCID, &fund.CategoryID,
		&fund.ISINGrowth, &isActive, &fund.CreatedAt, &fund.UpdatedAt,
		&fund.AMCName, &fund.CategoryName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get fund: %w", err)
	}
	fund.IsActive = isActive == 1
	return &fund, nil
}

func (r *FundRepository) ListFunds(ctx context.Context) ([]models.FundWithDetails, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT f.id, f.scheme_code, f.scheme_name, f.amc_id, f.category_id, 
		       COALESCE(f.isin_growth, ''), f.is_active, f.created_at, f.updated_at,
		       a.name, c.name
		FROM funds f
		JOIN amcs a ON f.amc_id = a.id
		JOIN categories c ON f.category_id = c.id
		WHERE f.is_active = 1
		ORDER BY a.name, c.name, f.scheme_name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list funds: %w", err)
	}
	defer rows.Close()

	var funds []models.FundWithDetails
	for rows.Next() {
		var fund models.FundWithDetails
		var isActive int
		if err := rows.Scan(
			&fund.ID, &fund.SchemeCode, &fund.SchemeName, &fund.AMCID, &fund.CategoryID,
			&fund.ISINGrowth, &isActive, &fund.CreatedAt, &fund.UpdatedAt,
			&fund.AMCName, &fund.CategoryName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan fund: %w", err)
		}
		fund.IsActive = isActive == 1
		funds = append(funds, fund)
	}
	return funds, rows.Err()
}

func (r *FundRepository) GetLatestNAV(ctx context.Context, fundID int64) (*models.NAVHistory, error) {
	var nav models.NAVHistory
	var navValue float64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, fund_id, date, nav, created_at
		FROM nav_history
		WHERE fund_id = ?
		ORDER BY date DESC
		LIMIT 1
	`, fundID).Scan(&nav.ID, &nav.FundID, &nav.Date, &navValue, &nav.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest NAV: %w", err)
	}
	nav.NAV = decimal.NewFromFloat(navValue)
	return &nav, nil
}

func (r *FundRepository) GetNAVHistory(ctx context.Context, fundID int64, startDate, endDate time.Time) ([]models.NAVHistory, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, fund_id, date, nav, created_at
		FROM nav_history
		WHERE fund_id = ? AND date >= ? AND date <= ?
		ORDER BY date ASC
	`, fundID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to get NAV history: %w", err)
	}
	defer rows.Close()

	var history []models.NAVHistory
	for rows.Next() {
		var nav models.NAVHistory
		var navValue float64
		if err := rows.Scan(&nav.ID, &nav.FundID, &nav.Date, &navValue, &nav.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan NAV: %w", err)
		}
		nav.NAV = decimal.NewFromFloat(navValue)
		history = append(history, nav)
	}
	return history, rows.Err()
}

func (r *FundRepository) InsertNAVBatch(ctx context.Context, navs []models.NAVHistory) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO nav_history (fund_id, date, nav)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	var inserted int64
	for _, nav := range navs {
		result, err := stmt.ExecContext(ctx, nav.FundID, nav.Date.Format("2006-01-02"), nav.NAV.InexactFloat64())
		if err != nil {
			return inserted, fmt.Errorf("failed to insert NAV: %w", err)
		}
		affected, _ := result.RowsAffected()
		inserted += affected
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return inserted, nil
}

func (r *FundRepository) GetAnalyticsWindows(ctx context.Context) ([]models.AnalyticsWindow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, code, days, label, is_active
		FROM analytics_windows
		WHERE is_active = 1
		ORDER BY days ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics windows: %w", err)
	}
	defer rows.Close()

	var windows []models.AnalyticsWindow
	for rows.Next() {
		var w models.AnalyticsWindow
		var isActive int
		if err := rows.Scan(&w.ID, &w.Code, &w.Days, &w.Label, &isActive); err != nil {
			return nil, fmt.Errorf("failed to scan window: %w", err)
		}
		w.IsActive = isActive == 1
		windows = append(windows, w)
	}
	return windows, rows.Err()
}

func (r *FundRepository) UpsertFundAnalytics(ctx context.Context, analytics *models.FundAnalytics) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO fund_analytics (fund_id, window_id, rolling_return, cagr, max_drawdown, 
		                            volatility, sharpe_ratio, calculated_at, data_start_date, 
		                            data_end_date, nav_data_point_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(fund_id, window_id) DO UPDATE SET
			rolling_return = excluded.rolling_return,
			cagr = excluded.cagr,
			max_drawdown = excluded.max_drawdown,
			volatility = excluded.volatility,
			sharpe_ratio = excluded.sharpe_ratio,
			calculated_at = excluded.calculated_at,
			data_start_date = excluded.data_start_date,
			data_end_date = excluded.data_end_date,
			nav_data_point_count = excluded.nav_data_point_count
	`, analytics.FundID, analytics.WindowID, analytics.RollingReturn.InexactFloat64(),
		analytics.CAGR.InexactFloat64(), analytics.MaxDrawdown.InexactFloat64(),
		analytics.Volatility.InexactFloat64(), analytics.SharpeRatio.InexactFloat64(),
		analytics.CalculatedAt, analytics.DataStartDate.Format("2006-01-02"),
		analytics.DataEndDate.Format("2006-01-02"), analytics.NAVDataPointCount)
	return err
}

func (r *FundRepository) GetFundAnalytics(ctx context.Context, fundID int64) ([]models.FundAnalyticsWithWindow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT fa.id, fa.fund_id, fa.window_id, fa.rolling_return, fa.cagr, fa.max_drawdown,
		       fa.volatility, fa.sharpe_ratio, fa.calculated_at, fa.data_start_date,
		       fa.data_end_date, fa.nav_data_point_count, aw.code, aw.label
		FROM fund_analytics fa
		JOIN analytics_windows aw ON fa.window_id = aw.id
		WHERE fa.fund_id = ?
		ORDER BY aw.days ASC
	`, fundID)
	if err != nil {
		return nil, fmt.Errorf("failed to get fund analytics: %w", err)
	}
	defer rows.Close()

	var analytics []models.FundAnalyticsWithWindow
	for rows.Next() {
		var a models.FundAnalyticsWithWindow
		var rr, cagr, md, vol, sr float64
		if err := rows.Scan(
			&a.ID, &a.FundID, &a.WindowID, &rr, &cagr, &md, &vol, &sr,
			&a.CalculatedAt, &a.DataStartDate, &a.DataEndDate, &a.NAVDataPointCount,
			&a.WindowCode, &a.WindowLabel,
		); err != nil {
			return nil, fmt.Errorf("failed to scan analytics: %w", err)
		}
		a.RollingReturn = decimal.NewFromFloat(rr)
		a.CAGR = decimal.NewFromFloat(cagr)
		a.MaxDrawdown = decimal.NewFromFloat(md)
		a.Volatility = decimal.NewFromFloat(vol)
		a.SharpeRatio = decimal.NewFromFloat(sr)
		analytics = append(analytics, a)
	}
	return analytics, rows.Err()
}

func (r *FundRepository) GetAllFundsAnalytics(ctx context.Context, windowCode string) ([]struct {
	Fund      models.FundWithDetails
	Analytics models.FundAnalyticsWithWindow
}, error) {
	query := `
		SELECT f.id, f.scheme_code, f.scheme_name, f.amc_id, f.category_id, 
		       COALESCE(f.isin_growth, ''), f.is_active, f.created_at, f.updated_at,
		       a.name, c.name,
		       fa.id, fa.fund_id, fa.window_id, fa.rolling_return, fa.cagr, fa.max_drawdown,
		       fa.volatility, fa.sharpe_ratio, fa.calculated_at, fa.data_start_date,
		       fa.data_end_date, fa.nav_data_point_count, aw.code, aw.label
		FROM funds f
		JOIN amcs a ON f.amc_id = a.id
		JOIN categories c ON f.category_id = c.id
		LEFT JOIN fund_analytics fa ON f.id = fa.fund_id
		LEFT JOIN analytics_windows aw ON fa.window_id = aw.id
		WHERE f.is_active = 1`

	var args []interface{}
	if windowCode != "" {
		query += " AND aw.code = ?"
		args = append(args, windowCode)
	}
	query += " ORDER BY fa.cagr DESC NULLS LAST"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get all funds analytics: %w", err)
	}
	defer rows.Close()

	var results []struct {
		Fund      models.FundWithDetails
		Analytics models.FundAnalyticsWithWindow
	}

	for rows.Next() {
		var fund models.FundWithDetails
		var analytics models.FundAnalyticsWithWindow
		var isActive int
		var faID, faFundID, faWindowID sql.NullInt64
		var rr, cagr, md, vol, sr sql.NullFloat64
		var calcAt, startDate, endDate sql.NullTime
		var navCount sql.NullInt64
		var wCode, wLabel sql.NullString

		if err := rows.Scan(
			&fund.ID, &fund.SchemeCode, &fund.SchemeName, &fund.AMCID, &fund.CategoryID,
			&fund.ISINGrowth, &isActive, &fund.CreatedAt, &fund.UpdatedAt,
			&fund.AMCName, &fund.CategoryName,
			&faID, &faFundID, &faWindowID, &rr, &cagr, &md, &vol, &sr,
			&calcAt, &startDate, &endDate, &navCount, &wCode, &wLabel,
		); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}

		fund.IsActive = isActive == 1

		if faID.Valid {
			analytics.ID = faID.Int64
			analytics.FundID = faFundID.Int64
			analytics.WindowID = faWindowID.Int64
			analytics.RollingReturn = decimal.NewFromFloat(rr.Float64)
			analytics.CAGR = decimal.NewFromFloat(cagr.Float64)
			analytics.MaxDrawdown = decimal.NewFromFloat(md.Float64)
			analytics.Volatility = decimal.NewFromFloat(vol.Float64)
			analytics.SharpeRatio = decimal.NewFromFloat(sr.Float64)
			if calcAt.Valid {
				analytics.CalculatedAt = calcAt.Time
			}
			if startDate.Valid {
				analytics.DataStartDate = startDate.Time
			}
			if endDate.Valid {
				analytics.DataEndDate = endDate.Time
			}
			if navCount.Valid {
				analytics.NAVDataPointCount = int(navCount.Int64)
			}
			analytics.WindowCode = wCode.String
			analytics.WindowLabel = wLabel.String
		}

		results = append(results, struct {
			Fund      models.FundWithDetails
			Analytics models.FundAnalyticsWithWindow
		}{Fund: fund, Analytics: analytics})
	}
	return results, rows.Err()
}

func (r *FundRepository) GetNAVCount(ctx context.Context, fundID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nav_history WHERE fund_id = ?", fundID).Scan(&count)
	return count, err
}
