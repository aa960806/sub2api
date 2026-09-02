package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestEnsureInvoiceEnabledInTxRequiresIndependentGate(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		gate       string
		wantReason string
		wantOK     bool
	}{
		{name: "missing gate", config: `{"enabled":true}`, wantReason: "INVOICE_DISABLED"},
		{name: "legacy true namespaced false", config: `{"enabled":true}`, gate: "false", wantReason: "INVOICE_DISABLED"},
		{name: "non canonical gate", config: `{"enabled":true}`, gate: "TRUE", wantReason: "INVOICE_DISABLED"},
		{name: "both enabled", config: `{"enabled":true}`, gate: "true", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectBegin()
			rows := sqlmock.NewRows([]string{"key", "value"}).
				AddRow(service.SettingKeyInvoiceConfig, tt.config)
			if tt.gate != "" {
				rows.AddRow(service.SettingKeySubNexusInvoiceEnabled, tt.gate)
			}
			mock.ExpectQuery(regexp.QuoteMeta("SELECT key, value")).WillReturnRows(rows)
			if tt.wantOK {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}

			tx, err := db.BeginTx(context.Background(), nil)
			require.NoError(t, err)
			err = ensureInvoiceEnabledInTx(context.Background(), tx)
			if tt.wantOK {
				require.NoError(t, err)
				require.NoError(t, tx.Commit())
			} else {
				require.Equal(t, "INVOICE_DISABLED", infraerrors.Reason(err))
				require.NoError(t, tx.Rollback())
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
