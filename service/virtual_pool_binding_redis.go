package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const virtualPoolBindingRedisPrefix = "new-api:virtual-sticky:binding:v1:"

const virtualPoolBindingClaimScript = `
local current = redis.call('HMGET', KEYS[1], 'candidate', 'owner', 'generation', 'pending', 'expires_at')
local now = tonumber(ARGV[1])
local expires_at = tonumber(current[5] or '0')
if current[1] and expires_at > now then
  return {current[1], current[2], tonumber(current[3] or '0'), tonumber(current[4] or '1'), expires_at, 0}
end
local generation = tonumber(current[3] or '0') + 1
local expires = now + tonumber(ARGV[2])
redis.call('HSET', KEYS[1],
  'candidate', ARGV[3],
  'owner', ARGV[4],
  'generation', generation,
  'pending', 1,
  'expires_at', expires)
redis.call('PEXPIREAT', KEYS[1], math.ceil((expires + tonumber(ARGV[5])) * 1000))
return {ARGV[3], ARGV[4], generation, 1, expires, 1}
`

const virtualPoolBindingConfirmScript = `
local current = redis.call('HMGET', KEYS[1], 'candidate', 'owner', 'generation', 'pending', 'expires_at')
local now = tonumber(ARGV[1])
if not current[1] or tonumber(current[3] or '0') ~= tonumber(ARGV[2]) or current[2] ~= ARGV[3] or current[1] ~= ARGV[4] or tonumber(current[5] or '0') <= now then
  return 0
end
local expires = now + tonumber(ARGV[5])
redis.call('HSET', KEYS[1], 'pending', 0, 'expires_at', expires)
redis.call('PEXPIREAT', KEYS[1], math.ceil(expires * 1000))
return 1
`

const virtualPoolBindingRenewScript = `
local current = redis.call('HMGET', KEYS[1], 'candidate', 'owner', 'generation', 'pending', 'expires_at')
local now = tonumber(ARGV[1])
if not current[1] or tonumber(current[3] or '0') ~= tonumber(ARGV[2]) or current[2] ~= ARGV[3] or tonumber(current[4] or '1') ~= 0 or tonumber(current[5] or '0') <= now then
  return 0
end
local expires = now + tonumber(ARGV[4])
redis.call('HSET', KEYS[1], 'expires_at', expires)
redis.call('PEXPIREAT', KEYS[1], math.ceil(expires * 1000))
return 1
`

const virtualPoolBindingRenewPendingScript = `
local current = redis.call('HMGET', KEYS[1], 'candidate', 'owner', 'generation', 'pending', 'expires_at')
local now = tonumber(ARGV[1])
if not current[1] or tonumber(current[3] or '0') ~= tonumber(ARGV[2]) or current[2] ~= ARGV[3] or tonumber(current[4] or '0') ~= 1 or tonumber(current[5] or '0') <= now then
  return 0
end
local expires = now + tonumber(ARGV[4])
redis.call('HSET', KEYS[1], 'expires_at', expires)
redis.call('PEXPIREAT', KEYS[1], math.ceil(expires * 1000))
return 1
`

const virtualPoolBindingReleaseScript = `
local current = redis.call('HMGET', KEYS[1], 'candidate', 'owner', 'generation', 'pending', 'expires_at')
local now = tonumber(ARGV[1])
if not current[1] or tonumber(current[3] or '0') ~= tonumber(ARGV[2]) or current[2] ~= ARGV[3] or tonumber(current[4] or '1') ~= 1 or tonumber(current[5] or '0') <= now then
  return 0
end
redis.call('DEL', KEYS[1])
return 1
`

const virtualPoolBindingInvalidateScript = `
local generation = redis.call('HGET', KEYS[1], 'generation')
local expires_at = redis.call('HGET', KEYS[1], 'expires_at')
if not generation or tonumber(generation) ~= tonumber(ARGV[1]) or tonumber(expires_at or '0') <= tonumber(ARGV[2]) then
  return 0
end
redis.call('DEL', KEYS[1])
return 1
`

type VirtualPoolRedisBindingStore struct {
	client *redis.Client
}

func NewVirtualPoolRedisBindingStore(client *redis.Client) *VirtualPoolRedisBindingStore {
	return &VirtualPoolRedisBindingStore{client: client}
}

func (store *VirtualPoolRedisBindingStore) Claim(
	ctx context.Context,
	sessionKey string,
	candidateKey string,
	owner string,
	now time.Time,
	pendingTTL time.Duration,
	confirmedTTL time.Duration,
) (VirtualPoolBinding, bool, error) {
	if store == nil || store.client == nil {
		return VirtualPoolBinding{}, false, ErrVirtualPoolStoreUnavailable
	}
	result, err := store.client.Eval(ctx, virtualPoolBindingClaimScript, []string{store.key(sessionKey)},
		now.UnixMilli(),
		pendingTTL.Milliseconds(),
		candidateKey,
		owner,
		confirmedTTL.Milliseconds(),
	).Result()
	if err != nil {
		return VirtualPoolBinding{}, false, err
	}
	values, err := redisStringSlice(result)
	if err != nil || len(values) < 6 {
		return VirtualPoolBinding{}, false, ErrVirtualPoolStoreUnavailable
	}
	generation, _ := strconv.ParseInt(values[2], 10, 64)
	pending, _ := strconv.ParseInt(values[3], 10, 64)
	expiresAtMillis, _ := strconv.ParseInt(values[4], 10, 64)
	claimed, _ := strconv.ParseInt(values[5], 10, 64)
	return VirtualPoolBinding{
		CandidateKey: values[0],
		Owner:        values[1],
		Generation:   generation,
		Pending:      pending == 1,
		ExpiresAt:    time.UnixMilli(expiresAtMillis),
	}, claimed == 1, nil
}

func (store *VirtualPoolRedisBindingStore) Confirm(
	ctx context.Context,
	sessionKey string,
	owner string,
	generation int64,
	candidateKey string,
	now time.Time,
	confirmedTTL time.Duration,
) bool {
	if store == nil || store.client == nil {
		return false
	}
	result, err := store.client.Eval(ctx, virtualPoolBindingConfirmScript, []string{store.key(sessionKey)},
		now.UnixMilli(),
		generation,
		owner,
		candidateKey,
		confirmedTTL.Milliseconds(),
	).Int64()
	return err == nil && result == 1
}

func (store *VirtualPoolRedisBindingStore) Renew(
	ctx context.Context,
	sessionKey string,
	owner string,
	generation int64,
	now time.Time,
	confirmedTTL time.Duration,
) bool {
	if store == nil || store.client == nil {
		return false
	}
	result, err := store.client.Eval(ctx, virtualPoolBindingRenewScript, []string{store.key(sessionKey)},
		now.UnixMilli(),
		generation,
		owner,
		confirmedTTL.Milliseconds(),
	).Int64()
	return err == nil && result == 1
}

func (store *VirtualPoolRedisBindingStore) RenewPending(
	ctx context.Context,
	sessionKey string,
	owner string,
	generation int64,
	now time.Time,
	pendingTTL time.Duration,
) bool {
	if store == nil || store.client == nil {
		return false
	}
	result, err := store.client.Eval(ctx, virtualPoolBindingRenewPendingScript, []string{store.key(sessionKey)},
		now.UnixMilli(),
		generation,
		owner,
		pendingTTL.Milliseconds(),
	).Int64()
	return err == nil && result == 1
}

func (store *VirtualPoolRedisBindingStore) Release(
	ctx context.Context,
	sessionKey string,
	owner string,
	generation int64,
	now time.Time,
) bool {
	if store == nil || store.client == nil {
		return false
	}
	result, err := store.client.Eval(ctx, virtualPoolBindingReleaseScript, []string{store.key(sessionKey)},
		now.UnixMilli(),
		generation,
		owner,
	).Int64()
	return err == nil && result == 1
}

func (store *VirtualPoolRedisBindingStore) Invalidate(
	ctx context.Context,
	sessionKey string,
	generation int64,
	now time.Time,
) bool {
	if store == nil || store.client == nil {
		return false
	}
	result, err := store.client.Eval(ctx, virtualPoolBindingInvalidateScript, []string{store.key(sessionKey)},
		generation,
		now.UnixMilli(),
	).Int64()
	return err == nil && result == 1
}

func (store *VirtualPoolRedisBindingStore) key(sessionKey string) string {
	return virtualPoolBindingRedisPrefix + strings.TrimSpace(sessionKey)
}

func redisStringSlice(value any) ([]string, error) {
	switch typed := value.(type) {
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			switch v := item.(type) {
			case string:
				result = append(result, v)
			case []byte:
				result = append(result, string(v))
			case int64:
				result = append(result, strconv.FormatInt(v, 10))
			default:
				return nil, ErrVirtualPoolStoreUnavailable
			}
		}
		return result, nil
	default:
		return nil, errors.New("unexpected redis script result")
	}
}
