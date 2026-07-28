package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"feed-videos-system/backend/internal/app"
	"feed-videos-system/backend/internal/config"
	"feed-videos-system/backend/internal/infrastructure/mysql"
	"feed-videos-system/backend/internal/transport/httpserver"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	db, err := mysql.Open(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("connect mysql: %v", err)
	}

	application := app.New(cfg, db)
	if err := application.Storage.EnsureDir(); err != nil {
		log.Fatalf("prepare storage: %v", err)
	}

	router := httpserver.NewRouter(application)
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("backend listening on %s", cfg.HTTPAddr)
		errCh <- server.ListenAndServe()
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		log.Printf("shutdown signal received: %s", sig)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	if err := application.Shutdown(ctx); err != nil {
		log.Printf("application shutdown error: %v", err)
	}
}
