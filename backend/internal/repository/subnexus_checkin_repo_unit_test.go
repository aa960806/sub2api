package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSubNexusCheckInRepositoryStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT current_streak, last_checkin_date FROM activity_checkin_streaks WHERE user_id=$1")).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"current_streak", "last_checkin_date"}).AddRow(3, time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT amount, created_at FROM activity_reward_logs WHERE user_id=$1 AND source='checkin' AND period=$2")).WithArgs(int64(7), "2026-09-02").WillReturnRows(sqlmock.NewRows([]string{"amount", "created_at"}).AddRow(0.25, time.Now()))
	rec, err := NewSubNexusCheckInRepository(db).Status(context.Background(), 7, "2026-09-02")
	if err != nil || !rec.CheckedIn || rec.Streak != 3 || rec.Amount != 0.25 {
		t.Fatalf("record=%+v err=%v", rec, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
