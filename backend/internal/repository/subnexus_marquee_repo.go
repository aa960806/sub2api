package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subNexusMarqueeRepository struct{ db *sql.DB }

func NewSubNexusMarqueeRepository(db *sql.DB) service.MarqueeRepository {
	return &subNexusMarqueeRepository{db: db}
}

func (r *subNexusMarqueeRepository) ListActiveAdmin(ctx context.Context, now time.Time, limit int) ([]service.MarqueeBroadcast, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, content, source, enabled, priority, start_at, end_at,
		       created_by, created_at, updated_at
		FROM activity_broadcasts
		WHERE source = $1
		  AND enabled = TRUE
		  AND (start_at IS NULL OR start_at <= $2)
		  AND (end_at IS NULL OR end_at >= $2)
		ORDER BY priority DESC, created_at DESC, id DESC
		LIMIT $3
	`, service.MarqueeSourceAdmin, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanMarqueeBroadcasts(rows)
}

func (r *subNexusMarqueeRepository) ListAdmin(ctx context.Context) ([]service.MarqueeBroadcast, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, content, source, enabled, priority, start_at, end_at,
		       created_by, created_at, updated_at
		FROM activity_broadcasts
		WHERE source = $1
		ORDER BY priority DESC, created_at DESC, id DESC
	`, service.MarqueeSourceAdmin)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanMarqueeBroadcasts(rows)
}

func (r *subNexusMarqueeRepository) CreateAdmin(ctx context.Context, input service.MarqueeBroadcastInput, adminID int64) (*service.MarqueeBroadcast, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO activity_broadcasts
			(title, content, source, enabled, priority, start_at, end_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, 0))
		RETURNING id, title, content, source, enabled, priority, start_at, end_at,
		          created_by, created_at, updated_at
	`, input.Title, input.Content, service.MarqueeSourceAdmin, input.Enabled, input.Priority,
		input.StartAt, input.EndAt, adminID)
	item, err := scanMarqueeBroadcast(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *subNexusMarqueeRepository) UpdateAdmin(ctx context.Context, id int64, input service.MarqueeBroadcastInput) (*service.MarqueeBroadcast, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE activity_broadcasts
		SET title = $2, content = $3, enabled = $4, priority = $5,
		    start_at = $6, end_at = $7, updated_at = NOW()
		WHERE id = $1 AND source = $8
		RETURNING id, title, content, source, enabled, priority, start_at, end_at,
		          created_by, created_at, updated_at
	`, id, input.Title, input.Content, input.Enabled, input.Priority, input.StartAt, input.EndAt,
		service.MarqueeSourceAdmin)
	item, err := scanMarqueeBroadcast(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *subNexusMarqueeRepository) DeleteAdmin(ctx context.Context, id int64) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM activity_broadcasts WHERE id = $1 AND source = $2`, id, service.MarqueeSourceAdmin)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

type marqueeScanner interface {
	Scan(dest ...any) error
}

func scanMarqueeBroadcast(scanner marqueeScanner) (service.MarqueeBroadcast, error) {
	var item service.MarqueeBroadcast
	var startAt, endAt sql.NullTime
	var createdBy sql.NullInt64
	if err := scanner.Scan(
		&item.ID, &item.Title, &item.Content, &item.Source, &item.Enabled, &item.Priority,
		&startAt, &endAt, &createdBy, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return item, err
	}
	if startAt.Valid {
		item.StartAt = &startAt.Time
	}
	if endAt.Valid {
		item.EndAt = &endAt.Time
	}
	if createdBy.Valid {
		item.CreatedBy = &createdBy.Int64
	}
	return item, nil
}

func scanMarqueeBroadcasts(rows *sql.Rows) ([]service.MarqueeBroadcast, error) {
	items := make([]service.MarqueeBroadcast, 0)
	for rows.Next() {
		item, err := scanMarqueeBroadcast(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
