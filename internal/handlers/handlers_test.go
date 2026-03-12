package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kreditbee/mf-analytics/internal/config"
	"github.com/kreditbee/mf-analytics/internal/repository"
	"github.com/kreditbee/mf-analytics/internal/services"
	"github.com/kreditbee/mf-analytics/pkg/logger"
)

func setupTestRouter(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	log := logger.NewNop()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:            "localhost",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Database: config.DatabaseConfig{
			Path:            ":memory:",
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: time.Minute,
		},
		MFAPI: config.MFAPIConfig{
			BaseURL:         "https://api.mfapi.in",
			RequestsPerSec:  2,
			RequestsPerMin:  50,
			RequestsPerHour: 300,
			BlockDuration:   5 * time.Minute,
			Timeout:         10 * time.Second,
		},
		Sync: config.SyncConfig{
			BackfillBatchSize: 10,
			RetryAttempts:     3,
			RetryDelay:        5 * time.Second,
		},
	}

	db, err := repository.NewSQLiteDB(context.Background(), cfg.Database, log)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	fundRepo := repository.NewFundRepository(db)
	syncRepo := repository.NewSyncRepository(db)
	rateLimitRepo := repository.NewRateLimitRepository(db)

	rateLimiter := services.NewRateLimiter(cfg.MFAPI, rateLimitRepo, log)
	mfapiClient := services.NewMFAPIClient(cfg.MFAPI, rateLimiter, log)
	syncService := services.NewSyncService(cfg.Sync, fundRepo, syncRepo, mfapiClient, log)
	analyticsService := services.NewAnalyticsService(fundRepo, log)
	fundService := services.NewFundService(fundRepo, syncService, analyticsService, log)

	_ = fundService.InitializeTrackedFunds(context.Background())

	router := NewRouter(cfg, fundService, syncService, analyticsService, log)
	server := httptest.NewServer(router)

	return server, func() {
		server.Close()
		db.Close()
	}
}

func TestHealthEndpoint(t *testing.T) {
	server, cleanup := setupTestRouter(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", body["status"])
	}
}

func TestListFundsEndpoint(t *testing.T) {
	server, cleanup := setupTestRouter(t)
	defer cleanup()

	start := time.Now()
	resp, err := http.Get(server.URL + "/api/v1/funds")
	latency := time.Since(start)

	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if latency > 200*time.Millisecond {
		t.Errorf("response time %v exceeds 200ms requirement", latency)
	}

	var body SuccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !body.Success {
		t.Error("expected success to be true")
	}

	funds, ok := body.Data.([]interface{})
	if !ok {
		t.Fatal("expected data to be an array")
	}

	if len(funds) != 10 {
		t.Errorf("expected 10 tracked funds, got %d", len(funds))
	}
}

func TestGetFundEndpoint(t *testing.T) {
	server, cleanup := setupTestRouter(t)
	defer cleanup()

	start := time.Now()
	resp, err := http.Get(server.URL + "/api/v1/funds/120505")
	latency := time.Since(start)

	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if latency > 200*time.Millisecond {
		t.Errorf("response time %v exceeds 200ms requirement", latency)
	}
}

func TestGetFundEndpoint_NotFound(t *testing.T) {
	server, cleanup := setupTestRouter(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/api/v1/funds/999999")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestGetFundEndpoint_InvalidCode(t *testing.T) {
	server, cleanup := setupTestRouter(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/api/v1/funds/invalid")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestGetFundAnalyticsEndpoint(t *testing.T) {
	server, cleanup := setupTestRouter(t)
	defer cleanup()

	start := time.Now()
	resp, err := http.Get(server.URL + "/api/v1/funds/120505/analytics")
	latency := time.Since(start)

	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if latency > 200*time.Millisecond {
		t.Errorf("response time %v exceeds 200ms requirement", latency)
	}
}

func TestRankFundsEndpoint(t *testing.T) {
	server, cleanup := setupTestRouter(t)
	defer cleanup()

	start := time.Now()
	resp, err := http.Get(server.URL + "/api/v1/funds/rank?window=1Y&sort_by=cagr&limit=5")
	latency := time.Since(start)

	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if latency > 200*time.Millisecond {
		t.Errorf("response time %v exceeds 200ms requirement", latency)
	}
}

func TestRankFundsEndpoint_InvalidWindow(t *testing.T) {
	server, cleanup := setupTestRouter(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/api/v1/funds/rank?window=2Y")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestSyncStatusEndpoint(t *testing.T) {
	server, cleanup := setupTestRouter(t)
	defer cleanup()

	start := time.Now()
	resp, err := http.Get(server.URL + "/api/v1/sync/status")
	latency := time.Since(start)

	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if latency > 200*time.Millisecond {
		t.Errorf("response time %v exceeds 200ms requirement", latency)
	}
}
