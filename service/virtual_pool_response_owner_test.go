package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVirtualPoolMemoryResponseOwnerExpiresAndFencesUnknownIDs(t *testing.T) {
	store := NewVirtualPoolMemoryResponseOwnerStore()
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 17, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(ctx, "scope-a", "resp_1", "candidate-a", now, time.Minute))
	owner, found, err := store.Lookup(ctx, "scope-a", "resp_1", now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "candidate-a", owner.CandidateKey)

	_, found, err = store.Lookup(ctx, "scope-a", "resp_missing", now.Add(time.Second))
	require.NoError(t, err)
	assert.False(t, found)

	_, found, err = store.Lookup(ctx, "scope-a", "resp_1", now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.False(t, found, "an expired owner mapping must not route a continuation")

	_, found, err = store.Lookup(ctx, "scope-b", "resp_1", now.Add(time.Second))
	require.NoError(t, err)
	assert.False(t, found, "another user/token/model scope must not resolve the mapping")
}

func TestVirtualPoolRedisResponseOwnerSharesMappingAcrossClients(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = clientA.Close()
		_ = clientB.Close()
	})
	storeA := NewVirtualPoolRedisResponseOwnerStore(clientA)
	storeB := NewVirtualPoolRedisResponseOwnerStore(clientB)
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 17, 30, 0, 0, time.UTC)

	require.NoError(t, storeA.Save(ctx, "scope-shared", "resp_shared", "candidate-b", now, time.Minute))
	owner, found, err := storeB.Lookup(ctx, "scope-shared", "resp_shared", now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "candidate-b", owner.CandidateKey)

	_, found, err = storeB.Lookup(ctx, "scope-other", "resp_shared", now.Add(time.Second))
	require.NoError(t, err)
	assert.False(t, found)
}
