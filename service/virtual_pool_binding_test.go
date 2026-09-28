package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVirtualPoolIntegrationRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := strings.TrimSpace(os.Getenv("VIRTUAL_POOL_TEST_REDIS_ADDR"))
	if addr == "" {
		t.Skip("VIRTUAL_POOL_TEST_REDIS_ADDR is not configured")
	}
	host := addr
	if index := strings.LastIndex(addr, ":"); index >= 0 {
		host = strings.Trim(addr[:index], "[]")
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		t.Fatalf("VIRTUAL_POOL_TEST_REDIS_ADDR must use a loopback host, got %q", addr)
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestVirtualPoolRedisIntegrationSharesAtomicGeneration(t *testing.T) {
	clientA := newVirtualPoolIntegrationRedisClient(t)
	clientB := redis.NewClient(&redis.Options{Addr: clientA.Options().Addr})
	t.Cleanup(func() { _ = clientB.Close() })
	storeA := NewVirtualPoolRedisBindingStore(clientA)
	storeB := NewVirtualPoolRedisBindingStore(clientB)
	ctx := context.Background()
	now := time.Now()
	sessionKey := fmt.Sprintf("integration-session-%d", now.UnixNano())
	t.Cleanup(func() { _ = clientA.Del(context.Background(), storeA.key(sessionKey)).Err() })

	first, claimed, err := storeA.Claim(ctx, sessionKey, "channel:1", "owner-a", now, time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)

	same, claimed, err := storeB.Claim(ctx, sessionKey, "channel:2", "owner-b", now.Add(time.Second), time.Minute, time.Hour)
	require.NoError(t, err)
	require.False(t, claimed)
	assert.Equal(t, first.Generation, same.Generation)
	assert.Equal(t, "channel:1", same.CandidateKey)
	require.True(t, storeB.Confirm(ctx, sessionKey, "owner-a", first.Generation, "channel:1", now.Add(2*time.Second), time.Hour))
	assert.False(t, storeA.Confirm(ctx, sessionKey, "owner-b", first.Generation, "channel:2", now.Add(3*time.Second), time.Hour))
}

func TestVirtualPoolMemoryBindingFencesOldOwners(t *testing.T) {
	store := NewVirtualPoolMemoryBindingStore()
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 10, 0, 0, 0, time.UTC)

	first, claimed, err := store.Claim(ctx, "session", "channel:1", "owner-a", now, time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, int64(1), first.Generation)

	same, claimed, err := store.Claim(ctx, "session", "channel:2", "owner-b", now.Add(time.Second), time.Minute, time.Hour)
	require.NoError(t, err)
	assert.False(t, claimed)
	assert.Equal(t, first.Generation, same.Generation)
	assert.Equal(t, "channel:1", same.CandidateKey)
	assert.Equal(t, "owner-a", same.Owner)

	require.True(t, store.Confirm(ctx, "session", "owner-a", first.Generation, "channel:1", now.Add(2*time.Second), time.Hour))
	assert.False(t, store.Release(ctx, "session", "owner-a", first.Generation, now.Add(3*time.Second)),
		"a confirmed binding cannot be released by an old attempt")

	expired, claimed, err := store.Claim(ctx, "session", "channel:2", "owner-b", now.Add(2*time.Hour), time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)
	assert.Greater(t, expired.Generation, first.Generation)
	assert.Equal(t, "channel:2", expired.CandidateKey)

	assert.False(t, store.Confirm(ctx, "session", "owner-a", first.Generation, "channel:1", now.Add(2*time.Hour), time.Hour),
		"an old success cannot overwrite a newer generation")
	assert.False(t, store.Release(ctx, "session", "owner-a", first.Generation, now.Add(2*time.Hour)),
		"an old release cannot remove a newer generation")
}

func TestVirtualPoolRedisBindingSharesAtomicGeneration(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = clientA.Close()
		_ = clientB.Close()
	})
	storeA := NewVirtualPoolRedisBindingStore(clientA)
	storeB := NewVirtualPoolRedisBindingStore(clientB)
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 11, 0, 0, 0, time.UTC)

	first, claimed, err := storeA.Claim(ctx, "shared-session", "channel:1", "owner-a", now, time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)

	second, claimed, err := storeB.Claim(ctx, "shared-session", "channel:2", "owner-b", now.Add(time.Second), time.Minute, time.Hour)
	require.NoError(t, err)
	assert.False(t, claimed)
	assert.Equal(t, first.Generation, second.Generation)
	assert.Equal(t, "channel:1", second.CandidateKey)

	require.True(t, storeB.Confirm(ctx, "shared-session", "owner-a", first.Generation, "channel:1", now.Add(2*time.Second), time.Hour))
	require.True(t, storeA.Renew(ctx, "shared-session", "owner-a", first.Generation, now.Add(3*time.Second), time.Hour))
	assert.False(t, storeA.Confirm(ctx, "shared-session", "owner-b", first.Generation, "channel:2", now.Add(4*time.Second), time.Hour))
}

func TestVirtualPoolBindingRejectsRedisWithoutClient(t *testing.T) {
	store := NewVirtualPoolRedisBindingStore(nil)
	_, _, err := store.Claim(context.Background(), "session", "candidate", "owner", time.Now(), time.Minute, time.Hour)
	require.Error(t, err)
}

func TestVirtualPoolMemoryBindingConfirmRejectsExpiredPendingLease(t *testing.T) {
	store := NewVirtualPoolMemoryBindingStore()
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 11, 30, 0, 0, time.UTC)

	binding, claimed, err := store.Claim(ctx, "expired-session", "channel:1", "owner-a", now, time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)

	assert.False(t, store.Confirm(ctx, "expired-session", "owner-a", binding.Generation, "channel:1", now.Add(2*time.Minute), time.Hour),
		"an expired pending lease cannot become confirmed")
}

func TestVirtualPoolMemoryBindingRequiresCandidateMatchForConfirmAndRenew(t *testing.T) {
	store := NewVirtualPoolMemoryBindingStore()
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 11, 45, 0, 0, time.UTC)

	binding, claimed, err := store.Claim(ctx, "candidate-match-session", "channel:1", "owner-a", now, time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)
	assert.False(t, store.Confirm(ctx, "candidate-match-session", "owner-a", binding.Generation, "channel:2", now.Add(time.Second), time.Hour))
	require.True(t, store.Confirm(ctx, "candidate-match-session", "owner-a", binding.Generation, "channel:1", now.Add(time.Second), time.Hour))
	assert.True(t, store.Renew(ctx, "candidate-match-session", "owner-a", binding.Generation, now.Add(2*time.Second), time.Hour))
}
