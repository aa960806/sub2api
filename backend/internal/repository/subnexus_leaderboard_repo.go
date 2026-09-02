package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// subNexusLeaderboardRepository serves display-only leaderboard aggregates.
// It deliberately has no mutation methods: leaderboard reads must never alter
// balances, usage rows, affiliate rows, or reward logs.
type subNexusLeaderboardRepository struct {
	db *sql.DB
}

func NewSubNexusLeaderboardRepository(db *sql.DB) service.LeaderboardRepository {
	return &subNexusLeaderboardRepository{db: db}
}

func (r *subNexusLeaderboardRepository) GetLeaderboard(
	ctx context.Context,
	start time.Time,
	end time.Time,
	limit int,
	source string,
	period string,
) ([]service.LeaderboardEntry, error) {
	if r == nil || r.db == nil {
		return nil, sql.ErrConnDone
	}
	return r.getLeaderboard(ctx, r.db, start, end, limit, source, period)
}

// GetLeaderboardTx is used by reward settlement so the fresh aggregate and
// the idempotent reward/balance writes share one database transaction.
func (r *subNexusLeaderboardRepository) GetLeaderboardTx(
	ctx context.Context,
	tx *sql.Tx,
	start time.Time,
	end time.Time,
	limit int,
	source string,
	period string,
) ([]service.LeaderboardEntry, error) {
	if r == nil || tx == nil {
		return nil, sql.ErrConnDone
	}
	return r.getLeaderboard(ctx, tx, start, end, limit, source, period)
}

type leaderboardQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (r *subNexusLeaderboardRepository) getLeaderboard(
	ctx context.Context,
	queryer leaderboardQueryer,
	start time.Time,
	end time.Time,
	limit int,
	source string,
	period string,
) ([]service.LeaderboardEntry, error) {
	rows, err := queryer.QueryContext(ctx, `
		WITH ranked AS (
			SELECT
				ul.user_id,
				COALESCE(NULLIF(u.email, ''), CONCAT('user-', ul.user_id::text)) AS email,
				COALESCE(SUM(ul.actual_cost), 0) AS usage,
				COUNT(*) AS requests,
				COALESCE(SUM(
					COALESCE(ul.input_tokens, 0) +
					COALESCE(ul.output_tokens, 0) +
					COALESCE(ul.cache_creation_tokens, 0) +
					COALESCE(ul.cache_read_tokens, 0) +
					COALESCE(ul.cache_creation_5m_tokens, 0) +
					COALESCE(ul.cache_creation_1h_tokens, 0)
				), 0) AS tokens
			FROM usage_logs ul
			JOIN users u ON u.id = ul.user_id
			WHERE ul.created_at >= $1
			  AND ul.created_at < $2
			  AND ul.actual_cost > 0
			  AND u.deleted_at IS NULL
			GROUP BY ul.user_id, u.email
		)
		SELECT
			ROW_NUMBER() OVER (ORDER BY usage DESC, requests DESC, user_id ASC) AS rank,
			user_id,
			email,
			usage,
			requests,
			tokens,
			EXISTS (
				SELECT 1
				FROM activity_reward_logs ar
				WHERE ar.source = $4
				  AND ar.period = $5
				  AND ar.user_id = ranked.user_id
			) AS rewarded
		FROM ranked
		ORDER BY usage DESC, requests DESC, user_id ASC
		LIMIT $3
	`, start, end, limit, source, period)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	entries := make([]service.LeaderboardEntry, 0)
	for rows.Next() {
		var entry service.LeaderboardEntry
		if err := rows.Scan(
			&entry.Rank,
			&entry.UserID,
			&entry.Email,
			&entry.Usage,
			&entry.Requests,
			&entry.Tokens,
			&entry.Rewarded,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *subNexusLeaderboardRepository) GetInviteLeaderboard(
	ctx context.Context,
	start time.Time,
	end time.Time,
	limit int,
) ([]service.InviteLeaderboardEntry, error) {
	if r == nil || r.db == nil {
		return nil, sql.ErrConnDone
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH ranked AS (
			SELECT
				ua.inviter_id AS user_id,
				COALESCE(NULLIF(u.email, ''), CONCAT('user-', ua.inviter_id::text)) AS email,
				COUNT(*) AS invite_count
			FROM user_affiliates ua
			JOIN users u ON u.id = ua.inviter_id
			WHERE ua.inviter_id IS NOT NULL
			  AND ua.created_at >= $1
			  AND ua.created_at < $2
			  AND u.deleted_at IS NULL
			GROUP BY ua.inviter_id, u.email
		)
		SELECT
			ROW_NUMBER() OVER (ORDER BY invite_count DESC, user_id ASC) AS rank,
			user_id,
			email,
			invite_count
		FROM ranked
		ORDER BY invite_count DESC, user_id ASC
		LIMIT $3
	`, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	entries := make([]service.InviteLeaderboardEntry, 0)
	for rows.Next() {
		var entry service.InviteLeaderboardEntry
		if err := rows.Scan(&entry.Rank, &entry.UserID, &entry.Email, &entry.InviteCount); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
