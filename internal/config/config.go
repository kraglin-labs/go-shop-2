package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL  string
	Port         string
	JWTSecret    string
	AdminAPIKey  string
	SupabaseURL  string
	SupabaseKey  string
	BrevoAPIKey  string
	AdminEmail   string
	FromEmail    string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: .env file not found, relying on environment variables")
	} else {
		log.Println("info: loaded .env file")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cfg := &Config{
		DatabaseURL:  require("DATABASE_URL"),
		Port:         port,
		JWTSecret:    require("JWT_SECRET"),
		AdminAPIKey:  require("ADMIN_API_KEY"),
		SupabaseURL:  require("SUPABASE_URL"),
		SupabaseKey:  require("SUPABASE_SERVICE_ROLE_KEY"),
		BrevoAPIKey:  require("BREVO_API_KEY"),
		AdminEmail:   require("ADMIN_EMAIL"),
		FromEmail:    require("FROM_EMAIL"),
	}

	if len(cfg.JWTSecret) < 32 {
		log.Fatalf("JWT_SECRET must be at least 32 characters")
	}
	if len(cfg.AdminAPIKey) < 32 {
		log.Fatalf("ADMIN_API_KEY must be at least 32 characters")
	}

	log.Println("info: config loaded successfully")
	return cfg
}

func require(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("FATAL: missing required env var: %s", key)
	}
	return v
}
