// dashboard/auth/secret_redis_test.go
package auth

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRedisClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

func TestEnsureSecretCreatesOnFirstCall(t *testing.T) {
	client, _ := newRedisClient(t)
	got, err := EnsureSecret(context.Background(), client, "test:secret")
	if err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}
	if len(got) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(got))
	}
}

func TestEnsureSecretReturnsExistingOnSecondCall(t *testing.T) {
	client, _ := newRedisClient(t)
	first, err := EnsureSecret(context.Background(), client, "test:secret")
	if err != nil {
		t.Fatalf("first EnsureSecret: %v", err)
	}
	second, err := EnsureSecret(context.Background(), client, "test:secret")
	if err != nil {
		t.Fatalf("second EnsureSecret: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("subsequent call should return same secret: %x vs %x", first, second)
	}
}

func TestEnsureSecretConcurrent(t *testing.T) {
	client, _ := newRedisClient(t)
	results := make([][]byte, 16)
	var wg sync.WaitGroup
	for i := range results {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := EnsureSecret(context.Background(), client, "test:secret")
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
				return
			}
			results[i] = b
		}()
	}
	wg.Wait()
	first := results[0]
	for i, b := range results[1:] {
		if !bytes.Equal(first, b) {
			t.Fatalf("concurrent results diverged at %d: %x vs %x", i+1, first, b)
		}
	}
}

func TestEnsureSecretRejectsBadInputs(t *testing.T) {
	client, _ := newRedisClient(t)
	if _, err := EnsureSecret(context.Background(), nil, "k"); err == nil {
		t.Fatal("expected error for nil client")
	}
	if _, err := EnsureSecret(context.Background(), client, ""); err == nil {
		t.Fatal("expected error for empty key")
	}
}
