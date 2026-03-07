package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bored/internal/ai"
	"bored/internal/api"
	"bored/internal/issues"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		port          = flag.Int("port", 8081, "Port to listen on")
		corsOrigins   = flag.String("cors", "http://localhost:3000,http://127.0.0.1:3000", "Comma-separated allowed CORS origins")
		dbPath        = flag.String("db", envOrDefault("BORED_DB_PATH", "data/bored.sqlite"), "SQLite database path")
		shutdownGrace = flag.Duration("shutdown-timeout", 10*time.Second, "Graceful shutdown timeout")
	)
	flag.Parse()

	analyzer, err := ai.NewAnalyzerFromEnv()
	if err != nil {
		return err
	}

	issueStore, err := issues.OpenSQLiteStore(context.Background(), *dbPath)
	if err != nil {
		return err
	}
	defer issueStore.Close()

	server := api.NewServer(api.ServerConfig{
		Port:        *port,
		CORSOrigins: api.ParseCSV(*corsOrigins),
		APIPrefixes: []string{"/api/v1", "/api"},
		Analyzer:    analyzer,
		IssueStore:  issueStore,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
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
