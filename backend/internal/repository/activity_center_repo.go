package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type activityCenterRepository struct {
	db *sql.DB
}

func NewActivityCenterRepository(db *sql.DB) service.ActivityCenterRepository {
	return &activityCenterRepository{db: db}
}

func (r *activityCenterRepository) ListVisible(ctx context.Context, now time.Time) ([]service.ActivityCenterItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, slug, title, subtitle, description, icon, cover_url, route_path, external_url,
		       action_label, activity_type, enabled, sort_order, start_at, end_at, metadata,
		       created_by, created_at, updated_at
		FROM activity_center_items
		WHERE activity_type = $1
		  AND enabled = TRUE
		  AND (start_at IS NULL OR start_at <= $2)
		  AND (end_at IS NULL OR end_at >= $2)
		ORDER BY sort_order ASC, created_at DESC, id DESC
	`, service.ActivityCenterTypeCustom, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanActivityCenterItems(rows)
}

func (r *activityCenterRepository) ListAdmin(ctx context.Context) ([]service.ActivityCenterItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, slug, title, subtitle, description, icon, cover_url, route_path, external_url,
		       action_label, activity_type, enabled, sort_order, start_at, end_at, metadata,
		       created_by, created_at, updated_at
		FROM activity_center_items
		WHERE activity_type = $1
		ORDER BY sort_order ASC, created_at DESC, id DESC
	`, service.ActivityCenterTypeCustom)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanActivityCenterItems(rows)
}

func (r *activityCenterRepository) Create(ctx context.Context, input service.ActivityCenterItemInput, adminID int64) (*service.ActivityCenterItem, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO activity_center_items
			(slug, title, subtitle, description, icon, cover_url, route_path, external_url,
			 action_label, activity_type, enabled, sort_order, start_at, end_at, metadata, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,NULLIF($16,0))
		RETURNING id, slug, title, subtitle, description, icon, cover_url, route_path, external_url,
		          action_label, activity_type, enabled, sort_order, start_at, end_at, metadata,
		          created_by, created_at, updated_at
	`, input.Slug, input.Title, input.Subtitle, input.Description, input.Icon, input.CoverURL,
		input.RoutePath, input.ExternalURL, input.ActionLabel, service.ActivityCenterTypeCustom,
		input.Enabled, input.SortOrder, input.StartAt, input.EndAt, string(input.Metadata), adminID)
	item, err := scanActivityCenterItem(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *activityCenterRepository) Update(ctx context.Context, id int64, input service.ActivityCenterItemInput) (*service.ActivityCenterItem, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE activity_center_items
		SET slug=$2, title=$3, subtitle=$4, description=$5, icon=$6, cover_url=$7,
		    route_path=$8, external_url=$9, action_label=$10, enabled=$11, sort_order=$12,
		    start_at=$13, end_at=$14, metadata=$15::jsonb, updated_at=NOW()
		WHERE id=$1 AND activity_type='custom'
		RETURNING id, slug, title, subtitle, description, icon, cover_url, route_path, external_url,
		          action_label, activity_type, enabled, sort_order, start_at, end_at, metadata,
		          created_by, created_at, updated_at
	`, id, input.Slug, input.Title, input.Subtitle, input.Description, input.Icon, input.CoverURL,
		input.RoutePath, input.ExternalURL, input.ActionLabel, input.Enabled, input.SortOrder,
		input.StartAt, input.EndAt, string(input.Metadata))
	item, err := scanActivityCenterItem(row)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *activityCenterRepository) Delete(ctx context.Context, id int64) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM activity_center_items WHERE id = $1 AND activity_type = 'custom'`, id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type activityCenterScanner interface {
	Scan(dest ...any) error
}

func scanActivityCenterItem(scanner activityCenterScanner) (service.ActivityCenterItem, error) {
	var item service.ActivityCenterItem
	var startAt sql.NullTime
	var endAt sql.NullTime
	var createdBy sql.NullInt64
	var metadata []byte
	if err := scanner.Scan(
		&item.ID, &item.Slug, &item.Title, &item.Subtitle, &item.Description, &item.Icon, &item.CoverURL,
		&item.RoutePath, &item.ExternalURL, &item.ActionLabel, &item.ActivityType, &item.Enabled, &item.SortOrder,
		&startAt, &endAt, &metadata, &createdBy, &item.CreatedAt, &item.UpdatedAt,
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
	if len(metadata) == 0 || !json.Valid(metadata) {
		metadata = []byte(`{}`)
	}
	item.Metadata = json.RawMessage(metadata)
	return item, nil
}

func scanActivityCenterItems(rows *sql.Rows) ([]service.ActivityCenterItem, error) {
	items := make([]service.ActivityCenterItem, 0)
	for rows.Next() {
		item, err := scanActivityCenterItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
