package repository

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAffiliateUserOverviewSQLIncludesMaturedFrozenQuota(t *testing.T) {
	query := strings.Join(strings.Fields(affiliateUserOverviewSQL), " ")

	require.Contains(t, query, "ua.aff_quota + COALESCE(matured.matured_frozen_quota, 0)")
	require.Contains(t, query, "frozen_until <= NOW()")
}

func TestAffiliateRecordQueriesUseLedgerAuditFields(t *testing.T) {
	source, err := os.ReadFile("affiliate_repo.go")
	require.NoError(t, err)
	content := string(source)

	require.Contains(t, content, "JOIN payment_orders po ON po.id = ual.source_order_id")
	require.Contains(t, content, "ual.amount::double precision")
	require.Contains(t, content, "ual.balance_after::double precision")
	require.NotContains(t, content, "parseAffiliateRebateAmount")
	require.NotContains(t, content, `"current_balance": "u.balance"`)
}

func TestQueryAffiliateEnabledTxFailsClosedForMissingOrMalformedGate(t *testing.T) {
	cases := []struct {
		name      string
		rows      *sqlmock.Rows
		queryErr  error
		want      bool
		wantError bool
	}{
		{name: "enabled", rows: sqlmock.NewRows([]string{"value"}).AddRow("true"), want: true},
		{name: "disabled", rows: sqlmock.NewRows([]string{"value"}).AddRow("false")},
		{name: "malformed", rows: sqlmock.NewRows([]string{"value"}).AddRow(" TRUE ")},
		{name: "missing", rows: sqlmock.NewRows([]string{"value"})},
		{name: "query error", queryErr: errors.New("settings unavailable"), wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, mock := newAffiliateSQLMockClient(t)
			query := `SELECT value FROM settings WHERE key = $1 FOR UPDATE`
			expectation := mock.ExpectQuery(regexp.QuoteMeta(query)).
				WithArgs(service.SettingKeyAffiliateEnabled)
			if tc.queryErr != nil {
				expectation.WillReturnError(tc.queryErr)
			} else {
				expectation.WillReturnRows(tc.rows)
			}

			got, err := queryAffiliateEnabledTx(context.Background(), client)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
