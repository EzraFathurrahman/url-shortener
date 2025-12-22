package limiter

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	Rdb    *redis.Client
	Limit  int64
	Window time.Duration
}

func New(rdb *redis.Client, limit int64, window time.Duration) *Limiter {
	return &Limiter{Rdb: rdb, Limit: limit, Window: window}
}

func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	n, err := l.Rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, nil
	}
	if n == 1 {
		if err := l.Rdb.Expire(ctx, key, l.Window).Err(); err != nil {
			return false, err
		}
	}
	return n <= l.Limit, nil
}
