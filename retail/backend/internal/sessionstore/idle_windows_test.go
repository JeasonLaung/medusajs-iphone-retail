package sessionstore

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Explicit read-only Windows rehearsal against the local Redis used by the
// preview. No session keys, CONFIG SET or other writes. A 60-65s idle timeout
// is required to reproduce the native service's closed pooled connection.
func TestLocalRedisIdleRehearsal(t *testing.T) {
	if os.Getenv("SESSION_REDIS_IDLE_REHEARSAL") != "1" {
		t.Skip("explicit local Windows Redis idle rehearsal required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 115*time.Second)
	defer cancel()
	_, fixed, err := Open(ctx, "redis://127.0.0.1:6379/0", "eurorefurb:idle-check:v1:")
	if err != nil {
		t.Fatal("local Redis unavailable")
	}
	defer fixed.Close()
	settings, err := fixed.ConfigGet(ctx, "timeout").Result()
	timeout, e := strconv.Atoi(settings["timeout"])
	if err != nil || e != nil || timeout < 60 || timeout > 65 {
		t.Fatal("rehearsal requires local Redis idle timeout 60-65s")
	}
	opts := *fixed.Options()
	opts.ConnMaxIdleTime = 30 * time.Minute
	// Options() exposes the initialized zero retry count; passing that zero to
	// NewClient would restore its default retries and conceal the stale socket.
	opts.MaxRetries = -1
	previous := redis.NewClient(&opts)
	defer previous.Close()
	if previous.Ping(ctx).Err() != nil {
		t.Fatal("baseline connection unavailable")
	}
	t.Log("Read-only idle comparison started; waiting 100s without session/key writes (including server idle-scan grace)")
	select {
	case <-time.After(100 * time.Second):
	case <-ctx.Done():
		t.Fatal("idle wait interrupted")
	}
	oldErr := previous.Ping(ctx).Err()
	newErr := fixed.Ping(ctx).Err()
	if oldErr == nil {
		t.Fatal("baseline did not reproduce the closed pooled socket")
	}
	if newErr != nil {
		t.Fatal("bounded idle pool failed its first request")
	}
	if previous.Ping(ctx).Err() != nil {
		t.Fatal("baseline dependency did not recover on second request")
	}
	t.Log("baseline_first_ping=false fixed_first_ping=true baseline_second_ping=true; MaxRetries remains disabled")
}
