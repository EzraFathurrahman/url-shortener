package main

import (
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/url-shortener/internal/limiter"
	"github.com/url-shortener/internal/redis"

	"github.com/url-shortener/internal/handlers"
)

func main() {
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")
	redisPass := getenv("REDIS_PASS", "")
	baseURL := getenv("BASE_URL", "http://localhost:3000")

	rdb, err := redis.New(redisAddr, redisPass, 0)
	if err != nil {
		log.Fatal("redis connect error: ", err)
	}
	defer rdb.Close()

	app := fiber.New()

	lim := limiter.New(rdb, 10, time.Minute)
	h := handlers.New(rdb, lim, baseURL, 24*time.Hour) // short link TTL 24 jam (ubah bebas)

	app.Post("/api/shorten", h.Shorten)
	app.Get("/api/stats/:code", h.Stats)
	app.Get("/healthcheck", h.HealthCheck)
	app.Get("/:code", h.Redirect)

	log.Println("listening on :3000")
	log.Fatal(app.Listen(":3000"))
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
