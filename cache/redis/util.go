package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

func SetStruct[T any](
	ctx context.Context, r redis.UniversalClient, key string, value T, duration time.Duration,
) error {
	v, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return r.Set(ctx, key, v, duration).Err()
}

func GetStruct[T any](ctx context.Context, r redis.UniversalClient, key string) (*T, error) {
	value, err := r.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}

		return nil, err
	}

	var result T
	err = json.Unmarshal([]byte(value), &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
