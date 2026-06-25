package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/leykun/bybit-backtester/internal/api"
	"github.com/leykun/bybit-backtester/internal/cache"
	"github.com/leykun/bybit-backtester/internal/store"
)

func main() {
	addr := envOr("ADDR", ":8080")
	webDir := envOr("WEB_DIR", "./web")
	databaseURL := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if redisURL == "" {
		log.Fatal("REDIS_URL is required")
	}

	ctx := context.Background()

	db, err := store.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	barsTTL := envDuration("REDIS_BARS_TTL", 168*time.Hour)
	resultTTL := envDuration("REDIS_RESULT_TTL", 24*time.Hour)

	rdb, err := cache.Connect(ctx, redisURL, barsTTL, resultTTL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	pool := api.NewPool(ctx, 4, 100, db, rdb)
	srv := api.NewServer(pool, webDir)

	log.Printf("bybit-backtester listening on %s (postgres + redis)", addr)
	if err := srv.Router().Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("invalid %s=%q, using default %s", key, v, def)
		return def
	}
	return d
}
