package service

import (
	"context"
	"sync"
)

type cpaStoreSyncContextKey string

// CPAStoreSyncer mirrors CPA-compatible data into an external auth/config store.
type CPAStoreSyncer interface {
	SyncAccountUpsert(ctx context.Context, account *Account) error
	SyncAccountDelete(ctx context.Context, account *Account) error
	SyncAPIKeyUpsert(ctx context.Context, apiKey *APIKey, owner *User) error
	SyncAPIKeyDelete(ctx context.Context, apiKey *APIKey, owner *User) error
}

const cpaStoreSyncSuppressedKey cpaStoreSyncContextKey = "cpa_store_sync_suppressed"

var (
	cpaStoreSyncerMu     sync.RWMutex
	globalCPAStoreSyncer CPAStoreSyncer
)

func WithCPAStoreSyncSuppressed(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, cpaStoreSyncSuppressedKey, true)
}

func CPAStoreSyncSuppressed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	suppressed, _ := ctx.Value(cpaStoreSyncSuppressedKey).(bool)
	return suppressed
}

func SetGlobalCPAStoreSyncer(syncer CPAStoreSyncer) {
	cpaStoreSyncerMu.Lock()
	globalCPAStoreSyncer = syncer
	cpaStoreSyncerMu.Unlock()
}

func GlobalCPAStoreSyncer() CPAStoreSyncer {
	cpaStoreSyncerMu.RLock()
	defer cpaStoreSyncerMu.RUnlock()
	return globalCPAStoreSyncer
}
