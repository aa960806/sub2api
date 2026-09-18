package service

import (
	"context"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent/intercept"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/stretchr/testify/require"
)

// Evaluate the real soft-delete interceptor against a recorded repository
// query, rather than asserting the shape of a context implementation.
type mediaSoftDeleteQuery struct{ filtered bool }

func (*mediaSoftDeleteQuery) Type() string                       { return "Account" }
func (*mediaSoftDeleteQuery) Limit(int)                          {}
func (*mediaSoftDeleteQuery) Offset(int)                         {}
func (*mediaSoftDeleteQuery) Unique(bool)                        {}
func (*mediaSoftDeleteQuery) Order(...func(*entsql.Selector))    {}
func (q *mediaSoftDeleteQuery) WhereP(...func(*entsql.Selector)) { q.filtered = true }

type mediaHistoryAccountRepository struct {
	AccountRepository
	account *Account
	deleted bool
}

func (r *mediaHistoryAccountRepository) GetByID(ctx context.Context, id int64) (*Account, error) {
	query := &mediaSoftDeleteQuery{}
	filter := mixins.SoftDeleteMixin{}.Interceptors()[0].(intercept.TraverseFunc)
	if err := filter(ctx, query); err != nil {
		return nil, err
	}
	if r.account == nil || r.account.ID != id || (r.deleted && query.filtered) {
		return nil, ErrAccountNotFound
	}
	return r.account, nil
}

func TestMediaHistoricalAccountReadDoesNotAdmitSoftDeletedAccount(t *testing.T) {
	ctx := context.Background()
	account := &Account{ID: 23, Platform: PlatformSeedance, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	repo := &mediaHistoryAccountRepository{account: account, deleted: true}
	gateway := &GatewayService{accountRepo: repo}
	historical, err := gateway.GetMediaTaskAccount(ctx, account.ID)
	require.NoError(t, err)
	require.Same(t, account, historical)
	_, err = gateway.GetMediaTaskAccountForCreate(ctx, account.ID)
	require.ErrorIs(t, err, ErrMediaAccountUnavailableForMedia)
	// Historical reads must not contaminate the original caller context.
	_, err = repo.GetByID(ctx, account.ID)
	require.ErrorIs(t, err, ErrAccountNotFound)
	repo.deleted = false
	current, err := gateway.GetMediaTaskAccountForCreate(ctx, account.ID)
	require.NoError(t, err)
	require.Same(t, account, current)
}

func TestMediaHistoricalAccountReadRetainsPlatformAndTypeBoundary(t *testing.T) {
	for _, account := range []*Account{
		{ID: 23, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{ID: 23, Platform: PlatformSeedance, Type: AccountTypeOAuth},
	} {
		gateway := &GatewayService{accountRepo: &mediaHistoryAccountRepository{account: account, deleted: true}}
		_, err := gateway.GetMediaTaskAccount(context.Background(), account.ID)
		require.ErrorIs(t, err, ErrMediaAccountUnavailableForMedia)
	}
	var gateway *GatewayService
	_, err := gateway.GetMediaTaskAccount(context.Background(), 23)
	require.ErrorIs(t, err, ErrMediaAccountUnavailableForMedia)
}
