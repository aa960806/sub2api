//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

var activityCenterTestColumns = []string{
	"id", "slug", "title", "subtitle", "description", "icon", "cover_url", "route_path", "external_url",
	"action_label", "activity_type", "enabled", "sort_order", "start_at", "end_at", "metadata",
	"created_by", "created_at", "updated_at",
}

func activityCenterTestRows() *sqlmock.Rows {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return sqlmock.NewRows(activityCenterTestColumns).AddRow(
		int64(7), "custom-card", "Custom card", "subtitle", "description", "gift", "",
		"/activities", "", "Open", service.ActivityCenterTypeCustom, true, 3, nil, nil,
		[]byte(`{"source":"test"}`), int64(11), now, now,
	)
}

func TestActivityCenterRepositoryListQueriesRestrictToCustomItems(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewActivityCenterRepository(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	mock.ExpectQuery(`(?s)SELECT id, slug, title.*FROM activity_center_items\s+WHERE activity_type = \$1\s+AND enabled = TRUE`).
		WithArgs(service.ActivityCenterTypeCustom, now).
		WillReturnRows(activityCenterTestRows())
	items, err := repo.ListVisible(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, service.ActivityCenterTypeCustom, items[0].ActivityType)

	mock.ExpectQuery(`(?s)SELECT id, slug, title.*FROM activity_center_items\s+WHERE activity_type = \$1\s+ORDER BY`).
		WithArgs(service.ActivityCenterTypeCustom).
		WillReturnRows(activityCenterTestRows())
	adminItems, err := repo.ListAdmin(context.Background())
	require.NoError(t, err)
	require.Len(t, adminItems, 1)
	require.Equal(t, service.ActivityCenterTypeCustom, adminItems[0].ActivityType)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityCenterRepositoryCreateAlwaysUsesCustomType(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewActivityCenterRepository(db)
	mock.ExpectQuery(`(?s)INSERT INTO activity_center_items.*activity_type.*RETURNING`).
		WithArgs(
			"custom-card", "Title", "", "", "gift", "", "/activities", "", "Open",
			service.ActivityCenterTypeCustom, true, 0, nil, nil, "{}", int64(5),
		).
		WillReturnRows(activityCenterTestRows())
	item, err := repo.Create(context.Background(), service.ActivityCenterItemInput{
		Slug:         "custom-card",
		Title:        "Title",
		Icon:         "gift",
		RoutePath:    "/activities",
		ActionLabel:  "Open",
		ActivityType: "ignored-by-repository",
		Enabled:      true,
		Metadata:     []byte(`{}`),
	}, 5)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityCenterRepositoryUpdateAndDeleteRestrictToCustomItems(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewActivityCenterRepository(db)
	input := service.ActivityCenterItemInput{
		Slug:        "updated-card",
		Title:       "Updated",
		Icon:        "gift",
		ActionLabel: "Open",
		Metadata:    []byte(`{}`),
		Enabled:     true,
	}
	mock.ExpectQuery(`(?s)UPDATE activity_center_items.*WHERE id=\$1 AND activity_type='custom'`).
		WithArgs(9, "updated-card", "Updated", "", "", "gift", "", "", "", "Open", true, 0, nil, nil, "{}").
		WillReturnRows(activityCenterTestRows())
	_, err = repo.Update(context.Background(), 9, input)
	require.NoError(t, err)

	mock.ExpectExec(`DELETE FROM activity_center_items WHERE id = \$1 AND activity_type = 'custom'`).
		WithArgs(9).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deleted, err := repo.Delete(context.Background(), 9)
	require.NoError(t, err)
	require.True(t, deleted)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityCenterRepositoryUpdateMapsNoRowsWithoutChangingQueryScope(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewActivityCenterRepository(db)
	mock.ExpectQuery(`(?s)UPDATE activity_center_items.*WHERE id=\$1 AND activity_type='custom'`).
		WithArgs(1, "missing", "Missing", "", "", "gift", "", "", "", "", true, 0, nil, nil, "{}").
		WillReturnError(sql.ErrNoRows)
	_, err = repo.Update(context.Background(), 1, service.ActivityCenterItemInput{
		Slug: "missing", Title: "Missing", Icon: "gift", Metadata: []byte(`{}`), Enabled: true,
	})
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityCenterRepositoryGatedCreateLocksSwitchBeforeMutation(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewActivityCenterRepository(db)
	gated, ok := repo.(interface {
		CreateWithGate(context.Context, service.ActivityCenterItemInput, int64) (*service.ActivityCenterItem, error)
	})
	require.True(t, ok)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT value FROM settings\s+WHERE key = \$1 FOR SHARE`).
		WithArgs(service.SettingKeySubNexusActivityCenterEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("true"))
	mock.ExpectQuery(`(?s)INSERT INTO activity_center_items.*RETURNING`).
		WithArgs(
			"custom-card", "Title", "", "", "gift", "", "", "", "Open",
			service.ActivityCenterTypeCustom, true, 0, nil, nil, "{}", int64(5),
		).
		WillReturnRows(activityCenterTestRows())
	mock.ExpectCommit()

	item, err := gated.CreateWithGate(context.Background(), service.ActivityCenterItemInput{
		Slug: "custom-card", Title: "Title", Icon: "gift", ActionLabel: "Open", Enabled: true,
		Metadata: []byte(`{}`),
	}, 5)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityCenterRepositoryGatedCreateRejectsDisabledSwitch(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewActivityCenterRepository(db)
	gated, ok := repo.(interface {
		CreateWithGate(context.Context, service.ActivityCenterItemInput, int64) (*service.ActivityCenterItem, error)
	})
	require.True(t, ok)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT value FROM settings\s+WHERE key = \$1 FOR SHARE`).
		WithArgs(service.SettingKeySubNexusActivityCenterEnabled).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("false"))
	mock.ExpectRollback()

	item, err := gated.CreateWithGate(context.Background(), service.ActivityCenterItemInput{Slug: "custom-card", Title: "Title"}, 5)
	require.ErrorIs(t, err, service.ErrActivityCenterDisabled)
	require.Nil(t, item)
	require.NoError(t, mock.ExpectationsWereMet())
}
