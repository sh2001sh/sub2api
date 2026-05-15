package service

import (
	"context"
	"sync"
)

// CPAStoreSyncer mirrors CPA-compatible data into an external auth/config store.
type CPAStoreSyncer interface {
	SyncAccountUpsert(ctx context.Context, account *Account) error
	SyncAccountDelete(ctx context.Context, account *Account) error
	SyncAPIKeyUpsert(ctx context.Context, apiKey *APIKey, owner *User) error
	SyncAPIKeyDelete(ctx context.Context, apiKey *APIKey, owner *User) error
}

var (
	cpaStoreSyncerMu     sync.RWMutex
	globalCPAStoreSyncer CPAStoreSyncer
)

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
