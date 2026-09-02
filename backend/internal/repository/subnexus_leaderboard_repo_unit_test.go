package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSubNexusLeaderboardRepositoryUsageAggregationContract(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Date(2026, 8, 31, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	end := start.AddDate(0, 0, 7)
	query := regexp.QuoteMeta("WITH ranked AS (")
	mock.ExpectQuery(query).
		WithArgs(start, end, 5, "leaderboard_week", "2026-W36").
		WillReturnRows(sqlmock.NewRows([]string{"rank", "user_id", "email", "usage", "requests", "tokens", "rewarded"}).
			AddRow(1, int64(7), "alice@example.com", 2.5, int64(3), int64(120), true))
	entries, err := NewSubNexusLeaderboardRepository(db).GetLeaderboard(context.Background(), start, end, 5, "leaderboard_week", "2026-W36")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Tokens != 120 || !entries[0].Rewarded {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSubNexusLeaderboardRepositoryInviteAggregationContract(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	mock.ExpectQuery(regexp.QuoteMeta("WITH ranked AS (")).
		WithArgs(start, end, 10).
		WillReturnRows(sqlmock.NewRows([]string{"rank", "user_id", "email", "invite_count"}).
			AddRow(1, int64(4), "invite@example.com", int64(9)))
	entries, err := NewSubNexusLeaderboardRepository(db).GetInviteLeaderboard(context.Background(), start, end, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].InviteCount != 9 {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
