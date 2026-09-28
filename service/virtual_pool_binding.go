package service

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrVirtualPoolStoreUnavailable = errors.New("virtual pool binding store is unavailable")

type VirtualPoolBinding struct {
	CandidateKey string
	Owner        string
	Generation   int64
	Pending      bool
	ExpiresAt    time.Time
}

type VirtualPoolBindingStore interface {
	Claim(ctx context.Context, sessionKey, candidateKey, owner string, now time.Time, pendingTTL, confirmedTTL time.Duration) (VirtualPoolBinding, bool, error)
	Confirm(ctx context.Context, sessionKey, owner string, generation int64, candidateKey string, now time.Time, confirmedTTL time.Duration) bool
	Renew(ctx context.Context, sessionKey, owner string, generation int64, now time.Time, confirmedTTL time.Duration) bool
	RenewPending(ctx context.Context, sessionKey, owner string, generation int64, now time.Time, pendingTTL time.Duration) bool
	Release(ctx context.Context, sessionKey, owner string, generation int64, now time.Time) bool
	Invalidate(ctx context.Context, sessionKey string, generation int64, now time.Time) bool
}

type VirtualPoolMemoryBindingStore struct {
	mu       sync.Mutex
	bindings map[string]VirtualPoolBinding
}

func NewVirtualPoolMemoryBindingStore() *VirtualPoolMemoryBindingStore {
	return &VirtualPoolMemoryBindingStore{bindings: make(map[string]VirtualPoolBinding)}
}

func (store *VirtualPoolMemoryBindingStore) Claim(
	_ context.Context,
	sessionKey string,
	candidateKey string,
	owner string,
	now time.Time,
	pendingTTL time.Duration,
	confirmedTTL time.Duration,
) (VirtualPoolBinding, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	current, exists := store.bindings[sessionKey]
	if exists && current.ExpiresAt.After(now) {
		return current, false, nil
	}
	generation := current.Generation + 1
	if generation <= 0 {
		generation = 1
	}
	binding := VirtualPoolBinding{
		CandidateKey: candidateKey,
		Owner:        owner,
		Generation:   generation,
		Pending:      true,
		ExpiresAt:    now.Add(pendingTTL),
	}
	store.bindings[sessionKey] = binding
	return binding, true, nil
}

func (store *VirtualPoolMemoryBindingStore) Confirm(
	_ context.Context,
	sessionKey string,
	owner string,
	generation int64,
	candidateKey string,
	now time.Time,
	confirmedTTL time.Duration,
) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	current, exists := store.bindings[sessionKey]
	if !exists || current.Generation != generation || current.Owner != owner || current.CandidateKey != candidateKey || !current.Pending || !current.ExpiresAt.After(now) {
		return false
	}
	current.Pending = false
	current.ExpiresAt = now.Add(confirmedTTL)
	store.bindings[sessionKey] = current
	return true
}

func (store *VirtualPoolMemoryBindingStore) Renew(
	_ context.Context,
	sessionKey string,
	owner string,
	generation int64,
	now time.Time,
	confirmedTTL time.Duration,
) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	current, exists := store.bindings[sessionKey]
	if !exists || current.Generation != generation || current.Owner != owner || current.Pending || !current.ExpiresAt.After(now) {
		return false
	}
	current.ExpiresAt = now.Add(confirmedTTL)
	store.bindings[sessionKey] = current
	return true
}

func (store *VirtualPoolMemoryBindingStore) RenewPending(
	_ context.Context,
	sessionKey string,
	owner string,
	generation int64,
	now time.Time,
	pendingTTL time.Duration,
) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	current, exists := store.bindings[sessionKey]
	if !exists || current.Generation != generation || current.Owner != owner || !current.Pending || !current.ExpiresAt.After(now) {
		return false
	}
	current.ExpiresAt = now.Add(pendingTTL)
	store.bindings[sessionKey] = current
	return true
}

func (store *VirtualPoolMemoryBindingStore) Release(
	_ context.Context,
	sessionKey string,
	owner string,
	generation int64,
	now time.Time,
) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	current, exists := store.bindings[sessionKey]
	if !exists || current.Generation != generation || current.Owner != owner || !current.Pending {
		return false
	}
	if !current.ExpiresAt.After(now) {
		return false
	}
	delete(store.bindings, sessionKey)
	return true
}

func (store *VirtualPoolMemoryBindingStore) Invalidate(
	_ context.Context,
	sessionKey string,
	generation int64,
	now time.Time,
) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	current, exists := store.bindings[sessionKey]
	if !exists || current.Generation != generation || !current.ExpiresAt.After(now) {
		return false
	}
	delete(store.bindings, sessionKey)
	return true
}
