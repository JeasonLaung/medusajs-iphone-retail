package sessionstore

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
)

// Open accepts explicit connection and application namespace only, not URL
// tuning parameters that could undo timeout/retry bounds. No env discovery,
// localhost fallback, schema writes, FLUSH or KEYS. Caller owns Close.
func Open(ctx context.Context, connection, prefix string) (*Store, *redis.Client, error) {
	u, err := url.Parse(connection)
	if err != nil || !prefixPattern.MatchString(prefix) || u == nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Hostname() == "" || (u.Scheme != "redis" && u.Scheme != "rediss") {
		return nil, nil, ErrInvalid
	}
	if _, _, err := net.SplitHostPort(u.Host); err != nil {
		return nil, nil, ErrInvalid
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, nil, ErrInvalid
	}
	opts, err := redis.ParseURL(connection)
	if err != nil || opts.DB < 0 || opts.DB > 15 {
		return nil, nil, ErrInvalid
	}
	opts.MaxRetries = -1
	opts.DialTimeout = time.Second
	opts.ReadTimeout = time.Second
	opts.WriteTimeout = time.Second
	opts.PoolTimeout = time.Second
	opts.PoolSize = 8
	opts.MaxActiveConns = 8
	opts.ContextTimeoutEnabled = true
	opts.Protocol = 2
	opts.DisableIdentity = true
	opts.MaintNotificationsConfig = &maintnotifications.Config{Mode: "disabled"}
	client := redis.NewClient(opts)
	ping, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if client.Ping(ping).Err() != nil {
		_ = client.Close()
		return nil, nil, ErrUnavailable
	}
	store, err := New(client, prefix)
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return store, client, nil
}
