// mqtt/dashboard/auth/secret_redis.go
package auth

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/redis/go-redis/v9"
)

// EnsureSecret returns the cluster-wide HMAC secret for dashboard session
// cookies. The secret is stored at the given Redis key. If absent, a fresh
// 32-byte secret is generated and SETNX-ed; on race loss, the winning
// value is read back. Returns at most 32 bytes.
func EnsureSecret(ctx context.Context, client *redis.Client, key string) ([]byte, error) {
	if client == nil {
		return nil, errors.New("auth: nil redis client")
	}
	if key == "" {
		return nil, errors.New("auth: empty key")
	}

	// First, try to read.
	if b, err := client.Get(ctx, key).Bytes(); err == nil {
		return b, nil
	} else if !errors.Is(err, redis.Nil) {
		return nil, err
	}

	// Key absent. Generate and SETNX.
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	ok, err := client.SetNX(ctx, key, buf, 0).Result()
	if err != nil {
		return nil, err
	}
	if ok {
		return buf, nil
	}

	// Lost the race; re-read the winning value.
	return client.Get(ctx, key).Bytes()
}
