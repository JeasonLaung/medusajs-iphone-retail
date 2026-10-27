package sessionstore

import (
	"context"
	"testing"
)

func TestInvalidConnectionConfigurationHasNoFallback(t *testing.T) {
	for _, connection := range []string{"", "http://localhost:6379", "redis://localhost", "redis://localhost:0", "redis://localhost:99999", "redis://localhost:6379/16", "redis://localhost:6379/0?max_retries=5", "redis://localhost:6379/0#secret"} {
		store, client, err := Open(context.Background(), connection, "eurorefurb:test:v1:")
		if err != ErrInvalid || store != nil || client != nil {
			t.Fatal("invalid connection accepted")
		}
	}
	for _, prefix := range []string{"", "medusa:", "eurorefurb:prod:", "eurorefurb:Test:v1:", "eurorefurb:ab:v1:", "eurorefurb:test:v1:*"} {
		if store, client, err := Open(context.Background(), "redis://127.0.0.1:6379/0", prefix); err != ErrInvalid || store != nil || client != nil {
			t.Fatal("invalid namespace accepted")
		}
	}
}
