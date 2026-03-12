package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/kreditbee/mf-analytics/internal/config"
	"github.com/kreditbee/mf-analytics/internal/handlers"
	"github.com/kreditbee/mf-analytics/internal/repository"
	"github.com/kreditbee/mf-analytics/internal/services"
	"github.com/kreditbee/mf-analytics/pkg/logger"
)

func main() {
	log, err := logger.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	cfg := config.Load()
	log.Info("configuration loaded",
		zap.String("database_path", cfg.Database.Path),
		zap.Int("server_port", cfg.Server.Port),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := repository.NewSQLiteDB(ctx, cfg.Database, log)
	if err != nil {
		log.Fatal("failed to initialize database", zap.Error(err))
	}
	defer db.Close()
	log.Info("database initialized")

	fundRepo := repository.NewFundRepository(db)
	syncRepo := repository.NewSyncRepository(db)
	rateLimitRepo := repository.NewRateLimitRepository(db)

	rateLimiter := services.NewRateLimiter(cfg.MFAPI, rateLimitRepo, log)
	mfapiClient := services.NewMFAPIClient(cfg.MFAPI, rateLimiter, log)
	syncService := services.NewSyncService(cfg.Sync, fundRepo, syncRepo, mfapiClient, log)
	analyticsService := services.NewAnalyticsService(fundRepo, log)
	fundService := services.NewFundService(fundRepo, syncService, analyticsService, log)

	if err := fundService.InitializeTrackedFunds(ctx); err != nil {
		log.Fatal("failed to initialize tracked funds", zap.Error(err))
	}
	log.Info("tracked funds initialized")

	router := handlers.NewRouter(cfg, fundService, syncService, analyticsService, log)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		log.Info("starting HTTP server", zap.String("address", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("server forced to shutdown", zap.Error(err))
	}

	log.Info("server stopped")
}
