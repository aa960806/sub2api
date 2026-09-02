package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type leaderboardSettingsStub struct {
	values map[string]string
	errs   map[string]error
}

func (s *leaderboardSettingsStub) GetValue(_ context.Context, key string) (string, error) {
	if err := s.errs[key]; err != nil {
		return "", err
	}
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (s *leaderboardSettingsStub) Get(_ context.Context, key string) (*Setting, error) {
	value, err := s.GetValue(context.Background(), key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}
func (*leaderboardSettingsStub) Set(context.Context, string, string) error { return nil }
func (*leaderboardSettingsStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("unexpected GetMultiple")
}
func (*leaderboardSettingsStub) SetMultiple(context.Context, map[string]string) error { return nil }
func (*leaderboardSettingsStub) GetAll(context.Context) (map[string]string, error) {
	return nil, errors.New("unexpected GetAll")
}
func (*leaderboardSettingsStub) Delete(context.Context, string) error { return nil }

type leaderboardRepoStub struct {
	usageCalls  int
	inviteCalls int
	start       time.Time
	end         time.Time
	limit       int
	source      string
	period      string
	usage       []LeaderboardEntry
	invites     []InviteLeaderboardEntry
}

func (r *leaderboardRepoStub) GetLeaderboard(_ context.Context, start, end time.Time, limit int, source, period string) ([]LeaderboardEntry, error) {
	r.usageCalls++
	r.start, r.end, r.limit, r.source, r.period = start, end, limit, source, period
	return r.usage, nil
}
func (r *leaderboardRepoStub) GetInviteLeaderboard(_ context.Context, start, end time.Time, limit int) ([]InviteLeaderboardEntry, error) {
	r.inviteCalls++
	r.start, r.end, r.limit = start, end, limit
	return r.invites, nil
}

func enabledLeaderboardSettings() *leaderboardSettingsStub {
	return &leaderboardSettingsStub{values: map[string]string{
		SettingKeySubNexusLeaderboardEnabled: "true",
		SettingKeySubNexusLeaderboardConfig:  `{"weekly_enabled":true,"weekly_top_n":3,"weekly_rewards":[1,0.5,0.25],"monthly_enabled":true,"monthly_top_n":2,"monthly_rewards":[5,2]}`,
	}, errs: map[string]error{}}
}

func TestLeaderboardDisabledFailsClosedBeforeRepository(t *testing.T) {
	repo := &leaderboardRepoStub{}
	settings := &leaderboardSettingsStub{values: map[string]string{}, errs: map[string]error{}}
	svc := NewLeaderboardService(repo, settings)
	if _, err := svc.GetLeaderboard(context.Background(), "week", 20, time.Now()); infraerrors.Reason(err) != "LEADERBOARD_DISABLED" {
		t.Fatalf("expected LEADERBOARD_DISABLED, got %v", err)
	}
	if _, err := svc.GetInviteLeaderboard(context.Background(), "week", 20, time.Now()); infraerrors.Reason(err) != "LEADERBOARD_DISABLED" {
		t.Fatalf("expected invite LEADERBOARD_DISABLED, got %v", err)
	}
	if repo.usageCalls != 0 || repo.inviteCalls != 0 {
		t.Fatalf("disabled service touched repository: usage=%d invites=%d", repo.usageCalls, repo.inviteCalls)
	}
}

func TestLeaderboardSwitchIsStrictAndIndependentFromLegacyConfig(t *testing.T) {
	for _, raw := range []string{"TRUE", "1", " true ", "yes"} {
		settings := enabledLeaderboardSettings()
		settings.values[SettingKeySubNexusLeaderboardEnabled] = raw
		svc := NewLeaderboardService(nil, settings)
		if svc.Config(context.Background()).Enabled {
			t.Fatalf("switch value %q unexpectedly enabled board", raw)
		}
	}
	settings := enabledLeaderboardSettings()
	settings.values[SettingKeySubNexusLeaderboardEnabled] = "true"
	settings.values[SettingKeySubNexusLeaderboardConfig] = `{"enabled":false,"weekly_top_n":2}`
	if !NewLeaderboardService(nil, settings).Config(context.Background()).Enabled {
		t.Fatal("independent switch should control a valid policy")
	}
	settings.values[SettingKeySubNexusLeaderboardConfig] = `null`
	if NewLeaderboardService(nil, settings).Config(context.Background()).Enabled {
		t.Fatal("null policy must fail closed")
	}
}

func TestLeaderboardWindowAndLimitNormalization(t *testing.T) {
	old := timezone.Name()
	_ = old // timezone.Init is intentionally not called; use the configured process zone.
	repo := &leaderboardRepoStub{usage: []LeaderboardEntry{{UserID: 7, Email: "alice@example.com", Usage: 1.25, Requests: 2, Tokens: 10}}}
	svc := NewLeaderboardService(repo, enabledLeaderboardSettings())
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, timezone.Location()) // Wednesday
	resp, err := svc.GetLeaderboard(context.Background(), "week", 999, now)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StartDate != "2026-08-31" || resp.EndDate != "2026-09-06" {
		t.Fatalf("unexpected week window: %s..%s", resp.StartDate, resp.EndDate)
	}
	if repo.limit != 20 {
		t.Fatalf("limit was not clamped to default: %d", repo.limit)
	}
	if len(resp.Entries) != 1 || resp.Entries[0].Email == "alice@example.com" {
		t.Fatalf("email was not masked: %+v", resp.Entries)
	}
	if resp.RewardTopN != 3 || resp.Entries[0].RewardAmount != 1 {
		t.Fatalf("reward metadata mismatch: %+v", resp)
	}
}

func TestLeaderboardInviteBoardIsReadOnlyAndMasksEmail(t *testing.T) {
	repo := &leaderboardRepoStub{invites: []InviteLeaderboardEntry{{UserID: 4, Email: "invite@example.com", InviteCount: 3}}}
	svc := NewLeaderboardService(repo, enabledLeaderboardSettings())
	resp, err := svc.GetInviteLeaderboard(context.Background(), "month", 10, time.Date(2026, 9, 10, 1, 0, 0, 0, timezone.Location()))
	if err != nil {
		t.Fatal(err)
	}
	if repo.inviteCalls != 1 || repo.usageCalls != 0 {
		t.Fatalf("unexpected repository calls: %+v", repo)
	}
	if len(resp.Entries) != 1 || resp.Entries[0].Email == "invite@example.com" || resp.Entries[0].InviteCount != 3 {
		t.Fatalf("unexpected invite response: %+v", resp)
	}
}
