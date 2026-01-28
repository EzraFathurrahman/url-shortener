package handlers

import (
	"context"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"github.com/url-shortener/internal/utils"

	"github.com/url-shortener/internal/limiter"
)

type Handler struct {
	Rdb     *redis.Client
	Limiter *limiter.Limiter
	BaseURL string
	URLTTL  time.Duration
}

type ShortenReq struct {
	LongURL string `json:"longUrl"`
}

type ShortenResp struct {
	Code     string `json:"code"`
	ShortURL string `json:"shortUrl"`
	LongURL  string `json:"longUrl"`
}

func New(rdb *redis.Client, lim *limiter.Limiter, baseURL string, ttl time.Duration) *Handler {
	return &Handler{Rdb: rdb, Limiter: lim, BaseURL: baseURL, URLTTL: ttl}
}

// POST /api/shorten
func (h *Handler) Shorten(c *fiber.Ctx) error {
	ctx := context.Background()

	ip := c.IP()
	rlKey := "rl:shorten:" + ip

	allowed, err := h.Limiter.Allow(ctx, rlKey)
	if err != nil {
		// Redis error -> fallback: allow (biar API ga mati), tapi ini tergantung policy lu
		allowed = true
	}
	if !allowed {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"error": "rate limit exceeded (10/min)",
		})
	}

	var req ShortenReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.LongURL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "longUrl is required"})
	}
	if _, err := url.ParseRequestURI(req.LongURL); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "longUrl is not a valid URL"})
	}

	// generate code & ensure uniqueness (simple retry)
	var code string
	for i := 0; i < 3; i++ {
		newCode, err := utils.NewCode(5) // ~7 chars
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to generate code"})
		}

		key := "short:" + newCode
		ok, err := h.Rdb.SetNX(ctx, key, req.LongURL, h.URLTTL).Result()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "redis error"})
		}
		if ok {
			code = newCode
			break
		}
	}
	if code == "" {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create unique code"})
	}

	resp := ShortenResp{
		Code:     code,
		ShortURL: h.BaseURL + "/" + code,
		LongURL:  req.LongURL,
	}
	return c.JSON(resp)
}

// GET /:code  -> redirect
func (h *Handler) Redirect(c *fiber.Ctx) error {
	ctx := context.Background()
	code := c.Params("code")
	if code == "" {
		return c.SendStatus(404)
	}

	key := "short:" + code
	longURL, err := h.Rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return c.Status(404).JSON(fiber.Map{"error": "code not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "redis error"})
	}

	// hit counter (best-effort)
	_ = h.Rdb.Incr(ctx, "hits:"+code).Err()

	return c.Redirect(longURL, fiber.StatusFound)
}

// GET /api/stats/:code
func (h *Handler) Stats(c *fiber.Ctx) error {
	ctx := context.Background()
	code := c.Params("code")

	// check exists
	longURL, err := h.Rdb.Get(ctx, "short:"+code).Result()
	if err == redis.Nil {
		return c.Status(404).JSON(fiber.Map{"error": "code not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "redis error"})
	}

	hits, err := h.Rdb.Get(ctx, "hits:"+code).Int64()
	if err == redis.Nil {
		hits = 0
	} else if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "redis error"})
	}

	return c.JSON(fiber.Map{
		"code":    code,
		"longUrl": longURL,
		"hits":    hits,
	})
}

// GET /health
func (h *Handler) HealthCheck(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusOK)
}
