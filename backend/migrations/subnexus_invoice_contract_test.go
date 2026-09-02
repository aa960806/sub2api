package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The invoice migration is intentionally an isolated, additive schema.  Keep
// this contract close to the embedded SQL so a future edit cannot silently
// turn the feature on or alter payment data during startup.
func TestSubNexusInvoiceMigrationContract(t *testing.T) {
	content, err := FS.ReadFile("9003_subnexus_invoice_transactions.sql")
	require.NoError(t, err)
	sql := strings.ToLower(stripSQLLineComments(string(content)))

	for _, want := range []string{
		"create table if not exists invoice_requests",
		"create table if not exists invoice_request_orders",
		"create table if not exists invoice_files",
		"create table if not exists invoice_audit_logs",
		"idx_invoice_request_orders_active_payment",
		"idx_invoice_files_current_request",
		"invoice_requests_status_check",
		"invoice_requests_title_fields_check",
		"invoice_requests_currency_check",
		"invoice_files_type_check",
	} {
		require.Contains(t, sql, want)
	}

	// Startup migration must never create an invoice request, reserve an order,
	// rewrite payment_orders, or enable the runtime switch.
	for _, forbidden := range []string{
		`\binsert\s+into\b`,
		`\bupdate\s+`,
		`\bdelete\s+from\b`,
		`\bdrop\s+`,
		`\btruncate\s+`,
		`\balter\s+table\s+payment_orders\b`,
		`\binvoice_enabled\b`,
		`\binvoice_config\b`,
	} {
		require.NotRegexp(t, regexp.MustCompile(`(?i)`+forbidden), sql,
			"invoice migration must remain additive and disabled by default")
	}
}
