package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-shop-2/internal/config"
	"go-shop-2/internal/db"
	"go-shop-2/internal/router"
)

func main() {
	cfg := config.Load()

	database, err := db.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connection failed: %v", err)
	}
	defer database.Close()

	r := router.New(database, cfg.JWTSecret, cfg.AdminAPIKey, cfg.SupabaseURL, cfg.SupabaseKey)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("Server running on http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("server stopped")
}
