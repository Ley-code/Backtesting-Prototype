// Command api starts the Bybit backtester: a Gin REST API backed by a worker
// pool, serving a single-page UI for running backtests and viewing results.
package main

import (
	"log"
	"os"

	"github.com/leykun/bybit-backtester/internal/api"
)

func main() {
	addr := envOr("ADDR", ":8080")
	cacheDir := envOr("CACHE_DIR", "./.cache")
	webDir := envOr("WEB_DIR", "./web")

	// 4 workers, queue depth 100. Tune for the deployment; the point is that
	// concurrency is capped so the box degrades gracefully under load.
	pool := api.NewPool(4, 100, cacheDir)
	srv := api.NewServer(pool, webDir)

	log.Printf("bybit-backtester listening on %s (cache=%s, web=%s)", addr, cacheDir, webDir)
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
