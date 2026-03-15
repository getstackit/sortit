package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"splat/internal/ai"
	"splat/internal/api"
	"splat/internal/auth"
	"splat/internal/issues"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		port          = flag.Int("port", 8081, "Port to listen on")
		corsOrigins   = flag.String("cors", "http://localhost:3000,http://127.0.0.1:3000", "Comma-separated allowed CORS origins")
		databaseURL   = flag.String("database-url", envOrDefault("SPLAT_DATABASE_URL", ""), "PostgreSQL connection string")
		shutdownGrace = flag.Duration("shutdown-timeout", 10*time.Second, "Graceful shutdown timeout")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	analyzer, err := ai.NewAnalyzerFromEnv()
	if err != nil {
		return err
	}

	issueStore, err := issues.OpenPostgresStore(context.Background(), *databaseURL)
	if err != nil {
		return err
	}
	defer issueStore.Close() //nolint:errcheck

	githubProvider, err := auth.NewGitHubProvider(auth.GitHubProviderConfig{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
	})
	if err != nil {
		return err
	}

	webOrigin := envOrDefault("SPLAT_WEB_ORIGIN", firstNonEmpty(api.ParseCSV(*corsOrigins)))
	authService, err := auth.NewService(auth.ServiceConfig{
		Store:      auth.NewStore(issueStore.DB()),
		Provider:   githubProvider,
		WebOrigin:  webOrigin,
		SessionTTL: 30 * 24 * time.Hour,
	})
	if err != nil {
		return err
	}

	server := api.NewServer(api.ServerConfig{
		Port:        *port,
		CORSOrigins: api.ParseCSV(*corsOrigins),
		APIPrefixes: []string{"/api/v1", "/api"},
		Logger:      logger,
		Analyzer:    analyzer,
		IssueStore:  issueStore,
		Auth:        authService,
	})
	if err := server.Initialize(context.Background()); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stop:
		logger.Info("shutting down", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), *shutdownGrace)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
