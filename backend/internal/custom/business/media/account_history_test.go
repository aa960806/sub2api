package media

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaSoftDeletedAccountSupportsHistoryButRejectsPinnedCreate(t *testing.T) {
	env := setupMediaEnv(t)
	env.resolver.deleted = map[int64]bool{1: true}
	account := env.resolver.accounts[1]
	task := &MediaTaskRecord{AccountID: 1, CredentialHash: mediaAccountFingerprint(account)}
	historical, err := env.handler.boundAccount(context.Background(), task)
	require.NoError(t, err)
	require.Same(t, account, historical)
	_, err = env.handler.selectAccount(context.Background(), currentAPIKey, "seedance2.0", 1)
	require.ErrorIs(t, err, ErrMediaAccountUnavailable)
	// Deletion must not make historical credential identity checks disappear.
	task.CredentialHash = "changed-credential-hash"
	_, err = env.handler.boundAccount(context.Background(), task)
	require.Error(t, err)
}
