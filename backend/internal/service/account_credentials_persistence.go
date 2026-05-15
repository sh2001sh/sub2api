package service

import "context"

type accountCredentialsUpdater interface {
	UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error
}

func persistAccountCredentials(ctx context.Context, repo AccountRepository, account *Account, credentials map[string]any) error {
	if repo == nil || account == nil {
		return nil
	}

	account.Credentials = cloneCredentials(credentials)
	var err error
	if updater, ok := any(repo).(accountCredentialsUpdater); ok {
		err = updater.UpdateCredentials(ctx, account.ID, account.Credentials)
	} else {
		err = repo.Update(ctx, account)
	}
	if err != nil {
		return err
	}

	if syncer := GlobalCPAStoreSyncer(); syncer != nil {
		if err := syncer.SyncAccountUpsert(ctx, account); err != nil {
			return err
		}
	}
	return nil
}

func cloneCredentials(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
