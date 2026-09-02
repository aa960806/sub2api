package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type pendingSignupRewardRepoStub struct {
	inviteRewardAffiliateRepo
	enqueueCalls    []AffiliateSignupRewardPending
	processCalls    []int64
	processResult   AffiliateSignupRewardProcessResult
	processErr      error
	reconcileCalls  int
	reconcileResult AffiliateSignupRewardReconcileResult
	reconcileErr    error
}

type atomicSignupRewardRepoStub struct {
	*pendingSignupRewardRepoStub
	atomicCalls int
	atomicErr   error
}

func (r *atomicSignupRewardRepoStub) BindInviterAndEnqueueSignupReward(_ context.Context, _, _ int64, _ AffiliateSignupRewardPending) (bool, int64, error) {
	r.atomicCalls++
	if r.atomicErr != nil {
		return false, 0, r.atomicErr
	}
	return true, 17, nil
}

func (r *pendingSignupRewardRepoStub) EnqueueSignupReward(_ context.Context, pending AffiliateSignupRewardPending) (int64, bool, error) {
	r.enqueueCalls = append(r.enqueueCalls, pending)
	return 17, true, nil
}

func (r *pendingSignupRewardRepoStub) ProcessSignupReward(_ context.Context, jobID int64) (AffiliateSignupRewardProcessResult, error) {
	r.processCalls = append(r.processCalls, jobID)
	return r.processResult, r.processErr
}

func (r *pendingSignupRewardRepoStub) ReconcileSignupRewards(_ context.Context, _ int) (AffiliateSignupRewardReconcileResult, error) {
	r.reconcileCalls++
	return r.reconcileResult, r.reconcileErr
}

func TestAffiliateSignupRewardPersistsPendingBeforeImmediateAttempt(t *testing.T) {
	repo := &pendingSignupRewardRepoStub{
		inviteRewardAffiliateRepo: inviteRewardAffiliateRepo{bindResult: true},
		processResult: AffiliateSignupRewardProcessResult{
			Applied:       true,
			Completed:     true,
			InviterID:     41,
			InviteeUserID: 99,
		},
	}
	settings := &inviteRewardSettingRepo{values: enabledInviteRewardSettings()}
	ctx := WithAffiliateSignupIP(context.Background(), "203.0.113.9")

	err := newInviteRewardAffiliateService(repo, settings).BindInviterByCode(ctx, 99, "INVITER1")

	require.NoError(t, err)
	require.Equal(t, 1, repo.bindCalls)
	require.Len(t, repo.enqueueCalls, 1)
	require.Equal(t, int64(41), repo.enqueueCalls[0].InviterID)
	require.Equal(t, int64(99), repo.enqueueCalls[0].InviteeUserID)
	require.Equal(t, 8.25, repo.enqueueCalls[0].InviterAmount)
	require.Equal(t, 2.5, repo.enqueueCalls[0].InviteeAmount)
	require.Equal(t, "203.0.113.9", repo.enqueueCalls[0].ClientIP)
	require.Equal(t, []int64{17}, repo.processCalls)
	require.Zero(t, repo.grantCalls, "pending-capable repositories must not bypass the durable queue")
}

func TestAffiliateSignupRewardImmediateFailureRemainsNonBlocking(t *testing.T) {
	repo := &pendingSignupRewardRepoStub{
		inviteRewardAffiliateRepo: inviteRewardAffiliateRepo{bindResult: true},
		processResult: AffiliateSignupRewardProcessResult{
			RetryScheduled: true,
			InviterID:      41,
			InviteeUserID:  99,
		},
		processErr: errors.New("database temporarily unavailable"),
	}
	settings := &inviteRewardSettingRepo{values: enabledInviteRewardSettings()}
	ctx := WithAffiliateSignupIP(context.Background(), "203.0.113.9")

	err := newInviteRewardAffiliateService(repo, settings).BindInviterByCode(ctx, 99, "INVITER1")

	// Binding remains the upstream success boundary; the durable row is left
	// for the scanner after the immediate attempt fails.
	require.NoError(t, err)
	require.Equal(t, 1, repo.bindCalls)
	require.Len(t, repo.enqueueCalls, 1)
	require.Len(t, repo.processCalls, 1)
}

func TestAffiliateSignupRewardAtomicImmediateFailureIsFailOpen(t *testing.T) {
	repo := &atomicSignupRewardRepoStub{
		pendingSignupRewardRepoStub: &pendingSignupRewardRepoStub{
			inviteRewardAffiliateRepo: inviteRewardAffiliateRepo{},
			processResult: AffiliateSignupRewardProcessResult{
				RetryScheduled: true,
				InviterID:      41,
				InviteeUserID:  99,
			},
			processErr: errors.New("database temporarily unavailable"),
		},
	}
	settings := &inviteRewardSettingRepo{values: enabledInviteRewardSettings()}
	ctx := WithAffiliateSignupIP(context.Background(), "203.0.113.9")

	err := newInviteRewardAffiliateService(repo, settings).BindInviterByCode(ctx, 99, "INVITER1")

	require.NoError(t, err)
	require.Equal(t, 1, repo.atomicCalls)
	require.Equal(t, []int64{17}, repo.processCalls)
	require.Zero(t, repo.bindCalls, "the atomic repository owns the binding")
	require.Zero(t, repo.grantCalls, "a durable pending repository must not bypass the queue")
}

func TestAffiliateSignupRewardReconcilerHonorsClosedGate(t *testing.T) {
	repo := &pendingSignupRewardRepoStub{}
	settings := &inviteRewardSettingRepo{values: map[string]string{
		SettingKeySubNexusInviteRewardsEnabled: "false",
	}}
	svc := newInviteRewardAffiliateService(repo, settings)

	result, err := svc.ReconcileAffiliateSignupRewardsOnce(context.Background(), 25)

	require.NoError(t, err)
	require.Equal(t, AffiliateSignupRewardReconcileResult{}, result)
	require.Zero(t, repo.reconcileCalls)
}

func TestAffiliateSignupRewardReconcilerInvalidatesAppliedUsers(t *testing.T) {
	repo := &pendingSignupRewardRepoStub{
		reconcileResult: AffiliateSignupRewardReconcileResult{
			Examined:        1,
			Applied:         1,
			Completed:       1,
			AffectedUserIDs: []int64{41, 99},
		},
	}
	settings := &inviteRewardSettingRepo{values: enabledInviteRewardSettings()}
	svc := newInviteRewardAffiliateService(repo, settings)

	result, err := svc.ReconcileAffiliateSignupRewardsOnce(context.Background(), 25)

	require.NoError(t, err)
	require.Equal(t, 1, repo.reconcileCalls)
	require.Equal(t, 1, result.Applied)
}
