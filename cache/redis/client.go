package redis

import (
	"context"
	"fmt"
	"github.com/manhrev/mkit/config"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

func NewClient(ctx context.Context, cfg *config.App) (*redis.Client, error) {
	var (
		redisCfg = cfg.Redis
		dsn      = fmt.Sprintf("%s:%s", redisCfg.Host, redisCfg.Port)
	)

	client := redis.NewClient(&redis.Options{
		Addr:     dsn,
		DB:       redisCfg.DB,
		Password: redisCfg.Password,
	})

	if cfg.Tracing.Enabled {
		if err := redisotel.InstrumentTracing(client); err != nil {
			return nil, fmt.Errorf("cannot enable tracing for redis: %w", err)
		}
	}

	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("cant ping redis server: %w", err)
	}

	return client, nil
}
