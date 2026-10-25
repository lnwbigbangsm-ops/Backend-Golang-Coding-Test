// Command api runs the user management REST and gRPC servers.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/adapter/inbound/grpcapi"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/adapter/inbound/httpapi"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/adapter/inbound/scheduler"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/adapter/outbound/mongodb"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/adapter/outbound/security"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/config"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// ctx is cancelled on SIGINT/SIGTERM, which starts graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ---- Outbound adapters ----
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	client, err := mongodb.Connect(connectCtx, cfg.MongoURI)
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		dctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Disconnect(dctx); err != nil {
			logger.Error("mongodb disconnect", "error", err)
		}
	}()

	repo := mongodb.NewUserRepository(client.Database(cfg.MongoDatabase))
	if err := repo.EnsureIndexes(ctx); err != nil {
		return err
	}
	tokens, err := security.NewJWTService(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)
	if err != nil {
		return err
	}
	hasher := security.NewBcryptHasher(cfg.BcryptCost)

	// ---- Core ----
	users := service.NewUserService(repo, hasher)
	auth, err := service.NewAuthService(users, repo, hasher, tokens)
	if err != nil {
		return err
	}

	// ---- Inbound adapters ----
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(users, auth, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	grpcServer := grpcapi.NewServer(users, auth, logger)
	grpcLis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Go(func() {
		scheduler.NewUserCountReporter(users, cfg.UserCountInterval, logger).Run(ctx)
	})
	wg.Go(func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	})
	wg.Go(func() {
		logger.Info("grpc server listening", "addr", cfg.GRPCAddr)
		if err := grpcServer.Serve(grpcLis); err != nil && !errors.Is(err, net.ErrClosed) {
			errCh <- err
		}
	})

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case runErr = <-errCh:
		logger.Error("server error, shutting down", "error", runErr)
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown", "error", err)
	}
	stopGRPC(shutdownCtx, grpcServer.GracefulStop, grpcServer.Stop)

	wg.Wait()
	logger.Info("shutdown complete")
	return runErr
}

// stopGRPC drains in-flight RPCs, forcing a stop if ctx expires first.
func stopGRPC(ctx context.Context, graceful, force func()) {
	done := make(chan struct{})
	go func() {
		graceful()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		force()
		<-done
	}
}
