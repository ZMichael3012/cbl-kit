package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// keyPrefix namespaces provider limiter buckets in Redis (CBL-003 section 7).
const keyPrefix = "rl:provider:"

// allowScript implements the shared bucket: it refills by the elapsed time,
// consumes one token or reports the wait. The caller clock is passed as an
// argument, so the script never calls TIME; all bucket values are integers in
// micro-units and microseconds.
var allowScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local interval = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local scale = tonumber(ARGV[4])
local tokens = tonumber(redis.call('HGET', KEYS[1], 'tokens'))
local updated = tonumber(redis.call('HGET', KEYS[1], 'updated'))
if tokens == nil or updated == nil then
  tokens = burst * scale
  updated = now
end
local cap = burst * scale
if now > updated then
  local elapsed = now - updated
  local steps = math.floor(elapsed / interval)
  local added = steps * scale
  local consumed = steps * interval
  local partial = elapsed - consumed
  if partial > 0 then
    local extra = math.floor(partial * scale / interval)
    added = added + extra
    consumed = consumed + math.floor(extra * interval / scale)
  end
  if added > 0 then
    tokens = tokens + added
    updated = updated + consumed
    if tokens >= cap then
      tokens = cap
      updated = now
    end
  end
end
local allowed = 0
local retry = 0
if tokens >= scale then
  tokens = tokens - scale
  allowed = 1
else
  retry = math.ceil((scale - tokens) * interval / scale)
  if updated > now then
    retry = retry + (updated - now)
  end
end
redis.call('HSET', KEYS[1], 'tokens', string.format('%d', tokens), 'updated', string.format('%d', updated))
local retention = burst * interval + (updated - now)
if retention < 0 then
  retention = 0
end
local ttl = math.ceil(retention / 1000) + 1000
redis.call('PEXPIRE', KEYS[1], ttl)
return {allowed, math.floor(retry), math.floor(tokens / scale)}
`)

// pauseScript empties the bucket and moves the baseline so the first token after
// the pause appears exactly when the pause elapses; pause arrives in
// microseconds, the TTL is milliseconds.
var pauseScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local pause = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local interval = tonumber(ARGV[4])
redis.call('HSET', KEYS[1], 'tokens', '0', 'updated', string.format('%d', now + pause - interval))
local ttl = math.ceil(burst * interval / 1000) + 1000 + math.floor(pause / 1000)
redis.call('PEXPIRE', KEYS[1], ttl)
return 1
`)

// Redis implements Limiter on a Redis server; the bucket state is shared by all
// replicas of a service, so provider limits hold under horizontal scaling.
type Redis struct {
	client redis.Scripter
	cfg    Config
	now    func() time.Time
}

// NewRedis creates a shared limiter with the system clock.
func NewRedis(client redis.Scripter, cfg Config) (*Redis, error) {
	return NewRedisWithClock(client, cfg, time.Now)
}

// NewRedisWithClock creates a shared limiter with an injected clock for tests.
func NewRedisWithClock(client redis.Scripter, cfg Config, now func() time.Time) (*Redis, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: nil redis client", ErrInvalidConfig)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if now == nil {
		return nil, fmt.Errorf("%w: nil clock", ErrInvalidConfig)
	}
	return &Redis{client: client, cfg: cfg, now: now}, nil
}

// Allow implements Limiter.
func (r *Redis) Allow(ctx context.Context, key string) (Decision, error) {
	if err := validateKey(key); err != nil {
		return Decision{}, err
	}
	values, err := allowScript.Run(ctx, r.client, []string{keyPrefix + key},
		r.now().UnixMicro(), r.cfg.RefillInterval.Microseconds(), r.cfg.Burst, tokenScale).Int64Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("ratelimit: redis allow: %w", err)
	}
	if len(values) != 3 {
		return Decision{}, fmt.Errorf("ratelimit: redis allow: unexpected reply of %d values", len(values))
	}
	return Decision{
		Allowed:    values[0] == 1,
		RetryAfter: time.Duration(values[1]) * time.Microsecond,
		Remaining:  int(values[2]),
	}, nil
}

// Pause implements Limiter.
func (r *Redis) Pause(ctx context.Context, key string, d time.Duration) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if d < 0 {
		return fmt.Errorf("%w: %s", ErrInvalidPause, d)
	}
	_, err := pauseScript.Run(ctx, r.client, []string{keyPrefix + key},
		r.now().UnixMicro(), d.Microseconds(), r.cfg.Burst, r.cfg.RefillInterval.Microseconds()).Result()
	if err != nil {
		return fmt.Errorf("ratelimit: redis pause: %w", err)
	}
	return nil
}
