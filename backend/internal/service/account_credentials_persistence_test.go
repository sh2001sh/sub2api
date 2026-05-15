//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type cpaStoreSyncerStub struct {
	upsertCalls int
	lastAccount *Account
	upsertErr   error
}

func (s *cpaStoreSyncerStub) SyncAccountUpsert(_ context.Context, account *Account) error {
	s.upsertCalls++
	s.lastAccount = account
	return s.upsertErr
}

func (s *cpaStoreSyncerStub) SyncAccountDelete(_ context.Context, _ *Account) error {
	return nil
}

func (s *cpaStoreSyncerStub) SyncAPIKeyUpsert(_ context.Context, _ *APIKey, _ *User) error {
	return nil
}

func (s *cpaStoreSyncerStub) SyncAPIKeyDelete(_ context.Context, _ *APIKey, _ *User) error {
	return nil
}

func TestPersistAccountCredentials_SyncsCPAStore(t *testing.T) {
	t.Cleanup(func() { SetGlobalCPAStoreSyncer(nil) })

	repo := &tokenRefreshAccountRepo{}
	account := &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "old-token",
		},
	}
	syncer := &cpaStoreSyncerStub{}
	SetGlobalCPAStoreSyncer(syncer)

	err := persistAccountCredentials(context.Background(), repo, account, map[string]any{
		"access_token": "new-token",
	})
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, 1, syncer.upsertCalls)
	require.Equal(t, "new-token", syncer.lastAccount.GetCredential("access_token"))
}

func TestPersistAccountCredentials_PropagatesCPAStoreError(t *testing.T) {
	t.Cleanup(func() { SetGlobalCPAStoreSyncer(nil) })

	repo := &tokenRefreshAccountRepo{}
	account := &Account{
		ID:       2,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
	}
	syncer := &cpaStoreSyncerStub{upsertErr: errors.New("push failed")}
	SetGlobalCPAStoreSyncer(syncer)

	err := persistAccountCredentials(context.Background(), repo, account, map[string]any{
		"access_token": "token",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "push failed")
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, 1, syncer.upsertCalls)
}
