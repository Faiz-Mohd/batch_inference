// Command server is the entrypoint for the batch inference service. It loads
// config, runs migrations, wires the layers (dependency injection), and starts
// the HTTP server and the background worker pool with graceful shutdown.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/example/batch-inference/internal/config"
	"github.com/example/batch-inference/internal/handler"
	"github.com/example/batch-inference/internal/inference"
	"github.com/example/batch-inference/internal/logging"
	"github.com/example/batch-inference/internal/repository/postgres"
	"github.com/example/batch-inference/internal/service"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(logger)
	logger.Info("configuration loaded",
		"port", cfg.Port,
		"worker_pool_size", cfg.WorkerPoolSize,
		"max_attempts", cfg.MaxAttempts,
		"log_level", cfg.LogLevel,
		"log_format", cfg.LogFormat,
	)

	// Root context cancelled on SIGINT/SIGTERM (App Platform sends SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Database + migrations.
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("connected to database")

	if err := postgres.Migrate(ctx, pool, logger); err != nil {
		return err
	}

	// Repositories.
	batchRepo := postgres.NewBatchRepository(pool)
	promptRepo := postgres.NewPromptRepository(pool)

	// Startup recovery: re-queue any prompts left mid-flight by a prior crash.
	if recovered, err := promptRepo.RecoverStuck(ctx); err != nil {
		return err
	} else if recovered > 0 {
		logger.Warn("recovered stuck prompts on startup", "count", recovered)
	}

	// Services.
	batchSvc := service.NewBatchService(batchRepo, cfg.MaxBatchSize, cfg.MaxPromptLen, cfg.MaxAttempts)
	inferClient := inference.New(cfg.InferenceURL, cfg.RequestTimeout)
	processor := service.NewProcessor(promptRepo, batchRepo, inferClient, service.ProcessorConfig{
		PoolSize:     cfg.WorkerPoolSize,
		ClaimBatch:   cfg.ClaimBatchSize,
		PollInterval: cfg.PollInterval,
		BaseBackoff:  cfg.BaseBackoff,
		MaxBackoff:   cfg.MaxBackoff,
	}, logger)

	// HTTP handlers + router.
	router := handler.Router{
		Batch:  handler.NewBatchHandler(batchSvc, int64(cfg.MaxPromptLen)*int64(cfg.MaxBatchSize)+(1<<20)),
		Health: handler.NewHealthHandler(batchRepo),
		Mock: handler.NewMockHandler(handler.MockConfig{
			RatePerSec: cfg.MockRatePerSec,
			Burst:      cfg.MockBurst,
			FailRate:   cfg.MockFailRate,
			MaxLatency: cfg.MockMaxLatency,
		}),
		Logger: logger,
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router.Build(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	var wg sync.WaitGroup

	// Background worker pool.
	wg.Add(1)
	go func() {
		defer wg.Done()
		processor.Run(ctx)
	}()

	// HTTP server.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or a fatal server error.
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		logger.Error("http server error", "err", err)
		stop()
	}

	// Graceful shutdown: stop accepting new HTTP requests, then let workers drain.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "err", err)
	}

	// ctx is already cancelled here, so the processor is draining; wait for it.
	wg.Wait()
	logger.Info("shutdown complete")
	return nil
}
