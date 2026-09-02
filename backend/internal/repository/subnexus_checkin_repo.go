package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subNexusCheckInRepository struct{ db *sql.DB }

func NewSubNexusCheckInRepository(db *sql.DB) service.CheckInRepository {
	return &subNexusCheckInRepository{db: db}
}

func (r *subNexusCheckInRepository) Status(ctx context.Context, userID int64, period string) (service.CheckInRecord, error) {
	var rec service.CheckInRecord
	err := r.db.QueryRowContext(ctx, `SELECT current_streak, last_checkin_date FROM activity_checkin_streaks WHERE user_id=$1`, userID).Scan(&rec.Streak, &rec.LastDate)
	if err != nil && err != sql.ErrNoRows {
		return rec, err
	}
	if err == sql.ErrNoRows {
		rec.Streak = 0
	}
	var amount float64
	var created time.Time
	err = r.db.QueryRowContext(ctx, `SELECT amount, created_at FROM activity_reward_logs WHERE user_id=$1 AND source='checkin' AND period=$2`, userID, period).Scan(&amount, &created)
	if err == nil {
		rec.CheckedIn, rec.Amount, rec.CheckedAt = true, amount, &created
	} else if err != sql.ErrNoRows {
		return rec, err
	}
	return rec, nil
}

func (r *subNexusCheckInRepository) Claim(ctx context.Context, userID int64, period string, day time.Time, amount float64, ip string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_checkin_streaks(user_id) VALUES($1) ON CONFLICT DO NOTHING`, userID); err != nil {
		return err
	}
	var streak int
	var last sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT current_streak,last_checkin_date FROM activity_checkin_streaks WHERE user_id=$1 FOR UPDATE`, userID).Scan(&streak, &last); err != nil {
		return err
	}
	if last.Valid && last.Time.Format("2006-01-02") == period {
		return service.ErrCheckInAlreadyClaimed
	}
	if !last.Valid || last.Time.Format("2006-01-02") != day.AddDate(0, 0, -1).Format("2006-01-02") {
		streak = 0
	}
	streak++
	if _, err = tx.ExecContext(ctx, `INSERT INTO activity_reward_logs(user_id,source,period,amount,note,ip) VALUES($1,'checkin',$2,$3,'daily check-in',$4)`, userID, period, amount, ip); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE users SET balance=balance+$1,total_recharged=total_recharged+$1,updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL`, amount, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `UPDATE activity_checkin_streaks SET current_streak=$1,last_checkin_date=$2,updated_at=NOW() WHERE user_id=$3`, streak, period, userID); err != nil {
		return err
	}
	return tx.Commit()
}
