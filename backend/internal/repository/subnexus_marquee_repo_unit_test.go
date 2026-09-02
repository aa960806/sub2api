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

var marqueeColumns = []string{
	"id", "title", "content", "source", "enabled", "priority", "start_at", "end_at",
	"created_by", "created_at", "updated_at",
}

func marqueeRows() *sqlmock.Rows {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return sqlmock.NewRows(marqueeColumns).AddRow(
		int64(7), "Notice", "Message", service.MarqueeSourceAdmin, true, 10,
		nil, nil, int64(2), now, now,
	)
}

func TestMarqueeRepositoryEveryQueryRestrictsSourceToAdmin(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewSubNexusMarqueeRepository(db)
	now := time.Now().UTC().Truncate(time.Microsecond)

	mock.ExpectQuery(`(?s)FROM activity_broadcasts\s+WHERE source = \$1\s+AND enabled = TRUE`).
		WithArgs(service.MarqueeSourceAdmin, now, 12).
		WillReturnRows(marqueeRows())
	items, err := repo.ListActiveAdmin(context.Background(), now, 12)
	require.NoError(t, err)
	require.Equal(t, service.MarqueeSourceAdmin, items[0].Source)

	mock.ExpectQuery(`(?s)FROM activity_broadcasts\s+WHERE source = \$1\s+ORDER BY`).
		WithArgs(service.MarqueeSourceAdmin).
		WillReturnRows(marqueeRows())
	_, err = repo.ListAdmin(context.Background())
	require.NoError(t, err)

	mock.ExpectQuery(`(?s)INSERT INTO activity_broadcasts.*source.*RETURNING`).
		WithArgs("Notice", "Message", service.MarqueeSourceAdmin, true, 4, nil, nil, int64(9)).
		WillReturnRows(marqueeRows())
	_, err = repo.CreateAdmin(context.Background(), service.MarqueeBroadcastInput{Title: "Notice", Content: "Message", Enabled: true, Priority: 4}, 9)
	require.NoError(t, err)

	mock.ExpectQuery(`(?s)UPDATE activity_broadcasts.*WHERE id = \$1 AND source = \$8`).
		WithArgs(int64(7), "Notice", "Updated", false, 2, nil, nil, service.MarqueeSourceAdmin).
		WillReturnError(sql.ErrNoRows)
	_, err = repo.UpdateAdmin(context.Background(), 7, service.MarqueeBroadcastInput{Title: "Notice", Content: "Updated", Priority: 2})
	require.ErrorIs(t, err, sql.ErrNoRows)

	mock.ExpectExec(`DELETE FROM activity_broadcasts WHERE id = \$1 AND source = \$2`).
		WithArgs(int64(7), service.MarqueeSourceAdmin).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deleted, err := repo.DeleteAdmin(context.Background(), 7)
	require.NoError(t, err)
	require.True(t, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}
