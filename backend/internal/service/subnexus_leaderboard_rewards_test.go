package service

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type leaderboardRewardRepoStub struct {
	entries []LeaderboardEntry
	calls   int
}

func (r *leaderboardRewardRepoStub) GetLeaderboard(context.Context, time.Time, time.Time, int, string, string) ([]LeaderboardEntry, error) {
	return r.entries, nil
}
func (r *leaderboardRewardRepoStub) GetInviteLeaderboard(context.Context, time.Time, time.Time, int) ([]InviteLeaderboardEntry, error) {
	return nil, nil
}
func (r *leaderboardRewardRepoStub) GetLeaderboardTx(context.Context, *sql.Tx, time.Time, time.Time, int, string, string) ([]LeaderboardEntry, error) {
	r.calls++
	return r.entries, nil
}

type leaderboardRewardAuthStub struct{ users []int64 }

func (s *leaderboardRewardAuthStub) InvalidateAuthCacheByKey(context.Context, string)    {}
func (s *leaderboardRewardAuthStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}
func (s *leaderboardRewardAuthStub) InvalidateAuthCacheByUserID(_ context.Context, id int64) {
	s.users = append(s.users, id)
}

type leaderboardRewardBillingStub struct{ done chan int64 }

func (s *leaderboardRewardBillingStub) InvalidateUserBalance(_ context.Context, id int64) error {
	select {
	case s.done <- id:
	default:
	}
	return nil
}

func TestLeaderboardPeriodKeyUsesLegacyZeroPadding(t *testing.T) {
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, timezone.Location())
	if got := leaderboardPeriodKey(LeaderboardWindowWeek, start); got != "2026-W02" {
		t.Fatalf("period key = %q, want 2026-W02", got)
	}
	parsedStart, parsedEnd, canonical, err := leaderboardPeriodBounds(LeaderboardWindowWeek, "2026-W2")
	if err != nil {
		t.Fatal(err)
	}
	if canonical != "2026-W02" || !parsedStart.Equal(start) || !parsedEnd.Equal(start.AddDate(0, 0, 7)) {
		t.Fatalf("parsed period mismatch: %v %v %q", parsedStart, parsedEnd, canonical)
	}
}

func TestGrantLeaderboardRewardsDisabledDoesNotTouchDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings := &leaderboardSettingsStub{values: map[string]string{SettingKeySubNexusLeaderboardEnabled: "false"}, errs: map[string]error{}}
	svc := NewLeaderboardServiceWithDependencies(&leaderboardRewardRepoStub{}, settings, db, nil, nil, nil)
	if _, _, err := svc.GrantLeaderboardRewardsForPeriod(context.Background(), LeaderboardWindowWeek, "2026-W02"); infraerrors.Reason(err) != "LEADERBOARD_DISABLED" {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newRewardService(t *testing.T) (*LeaderboardService, sqlmock.Sqlmock, *leaderboardRewardAuthStub, *leaderboardRewardBillingStub, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	settings := enabledLeaderboardSettings()
	repo := &leaderboardRewardRepoStub{entries: []LeaderboardEntry{{Rank: 1, UserID: 7}}}
	auth := &leaderboardRewardAuthStub{}
	billing := &leaderboardRewardBillingStub{done: make(chan int64, 1)}
	svc := NewLeaderboardServiceWithDependencies(repo, settings, db, auth, billing, nil)
	return svc, mock, auth, billing, func() { _ = db.Close() }
}

func expectRewardGateAndConfig(mock sqlmock.Sqlmock, gate string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT value FROM settings WHERE key=$1 FOR UPDATE")).
		WithArgs(SettingKeySubNexusLeaderboardEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(gate))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT value FROM settings WHERE key=$1 FOR SHARE")).
		WithArgs(SettingKeySubNexusLeaderboardConfig).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"weekly_enabled":true,"weekly_top_n":1,"weekly_rewards":[1],"monthly_enabled":true,"monthly_top_n":1,"monthly_rewards":[5]}`))
}

func TestGrantLeaderboardRewardsIsIdempotentAndUpdatesBothBalanceColumns(t *testing.T) {
	svc, mock, auth, billing, cleanup := newRewardService(t)
	defer cleanup()
	mock.ExpectBegin()
	expectRewardGateAndConfig(mock, "true")
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO activity_reward_logs")).
		WithArgs(int64(7), ActivitySourceLeaderboardWeek, "2026-W02", 1, float64(1), "week rank #1 reward").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE users")).
		WithArgs(float64(1), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	granted, total, err := svc.GrantLeaderboardRewardsForPeriod(context.Background(), LeaderboardWindowWeek, "2026-W02")
	if err != nil || granted != 1 || total != 1 {
		t.Fatalf("grant result = %d %.2f %v", granted, total, err)
	}
	if len(auth.users) != 1 || auth.users[0] != 7 {
		t.Fatalf("auth cache invalidation = %#v", auth.users)
	}
	select {
	case got := <-billing.done:
		if got != 7 {
			t.Fatalf("billing invalidation user = %d", got)
		}
	case <-time.After(time.Second):
		t.Fatal("billing cache was not invalidated")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGrantLeaderboardRewardsDuplicateDoesNotCreditAgain(t *testing.T) {
	svc, mock, auth, _, cleanup := newRewardService(t)
	defer cleanup()
	mock.ExpectBegin()
	expectRewardGateAndConfig(mock, "true")
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO activity_reward_logs")).
		WithArgs(int64(7), ActivitySourceLeaderboardWeek, "2026-W02", 1, float64(1), "week rank #1 reward").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	granted, total, err := svc.GrantLeaderboardRewardsForPeriod(context.Background(), LeaderboardWindowWeek, "2026-W02")
	if err != nil || granted != 0 || total != 0 || len(auth.users) != 0 {
		t.Fatalf("duplicate result = %d %.2f auth=%v err=%v", granted, total, auth.users, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGrantLeaderboardRewardsRollsBackWhenUserIsMissing(t *testing.T) {
	svc, mock, _, _, cleanup := newRewardService(t)
	defer cleanup()
	mock.ExpectBegin()
	expectRewardGateAndConfig(mock, "true")
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO activity_reward_logs")).
		WithArgs(int64(7), ActivitySourceLeaderboardWeek, "2026-W02", 1, float64(1), "week rank #1 reward").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE users")).
		WithArgs(float64(1), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	_, _, err := svc.GrantLeaderboardRewardsForPeriod(context.Background(), LeaderboardWindowWeek, "2026-W02")
	if infraerrors.Reason(err) != "USER_NOT_FOUND" {
		t.Fatalf("unexpected missing-user error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGrantLeaderboardRewardsRechecksGateInsideTransaction(t *testing.T) {
	svc, mock, _, _, cleanup := newRewardService(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT value FROM settings WHERE key=$1 FOR UPDATE")).
		WithArgs(SettingKeySubNexusLeaderboardEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectRollback()
	_, _, err := svc.GrantLeaderboardRewardsForPeriod(context.Background(), LeaderboardWindowWeek, "2026-W02")
	if infraerrors.Reason(err) != "LEADERBOARD_DISABLED" {
		t.Fatalf("unexpected gate error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListRewardHistoryFiltersLeaderboardSourcesAndMasksEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings := enabledLeaderboardSettings()
	svc := NewLeaderboardServiceWithDependencies(nil, settings, db, nil, nil, nil)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM activity_reward_logs WHERE source IN ($1, $2)")).
		WithArgs(ActivitySourceLeaderboardWeek, ActivitySourceLeaderboardMonth).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	created := time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ar.id, ar.user_id, COALESCE(u.email, ''), ar.source, ar.period, ar.rank, ar.amount, ar.note, ar.created_at")).
		WithArgs(ActivitySourceLeaderboardWeek, ActivitySourceLeaderboardMonth, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "email", "source", "period", "rank", "amount", "note", "created_at"}).
			AddRow(int64(9), int64(7), "alice@example.com", ActivitySourceLeaderboardWeek, "2026-W02", 1, 1.0, "week rank #1 reward", created))
	history, err := svc.ListRewardHistory(context.Background(), 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if history.Total != 2 || len(history.Items) != 1 || history.Items[0].Email == "alice@example.com" || history.Items[0].Source != ActivitySourceLeaderboardWeek {
		t.Fatalf("unexpected history: %+v", history)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
