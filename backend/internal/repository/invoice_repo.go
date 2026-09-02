package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type invoiceRepository struct {
	db *sql.DB
}

func NewInvoiceRepository(db *sql.DB) service.InvoiceRepository {
	return &invoiceRepository{db: db}
}

const invoiceRequestColumns = `
	id, request_no, user_id, user_email_snapshot, user_name_snapshot, status,
	title_type, title_name, taxpayer_id, recipient_email, recipient_phone,
	company_address, company_phone, bank_name, bank_account, invoice_item_name,
	currency, total_amount::text, order_count, user_note, admin_note, reject_reason,
	invoice_code, invoice_number, invoice_date, config_snapshot::text, revision,
	accepted_by, accepted_at, issued_by, issued_at, rejected_by, rejected_at,
	cancelled_at, voided_by, voided_at, created_at, updated_at`

const invoicePaymentOrderCurrencySQL = `CASE
	WHEN BTRIM(COALESCE(po.provider_snapshot->>'currency',''))='' THEN 'CNY'
	WHEN UPPER(BTRIM(po.provider_snapshot->>'currency')) ~ '^[A-Z]{3}$' THEN UPPER(BTRIM(po.provider_snapshot->>'currency'))
	ELSE ''
END`

type invoiceScanner interface {
	Scan(...any) error
}

type invoiceQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type lockedPaymentOrder struct {
	ID                int64
	OutTradeNo        string
	OrderType         string
	PaymentType       string
	PayAmount         string
	ProviderSnapshot  string
	Status            string
	RefundAmount      string
	RefundRequestedAt sql.NullTime
	RefundAt          sql.NullTime
	PaidAt            sql.NullTime
	CompletedAt       sql.NullTime
}

func (r *invoiceRepository) HasHistory(ctx context.Context, userID int64) (bool, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return false, nil
	}
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM invoice_requests WHERE user_id=$1)`, userID).Scan(&exists)
	return exists, err
}

func (r *invoiceRepository) ListEligibleOrders(ctx context.Context, userID int64, cfg service.InvoiceConfig, page, pageSize int, keyword string) (service.InvoiceEligibleOrdersResult, error) {
	result := service.InvoiceEligibleOrdersResult{Page: page, PageSize: pageSize, IneligibleReasons: map[string]int64{}}
	if r == nil || r.db == nil || userID <= 0 {
		return result, infraerrors.InternalServer("INVOICE_CONFIG_INVALID", "invoice repository is unavailable")
	}
	page, pageSize = invoicePagination(page, pageSize)
	result.Page, result.PageSize = page, pageSize
	keyword = strings.TrimSpace(keyword)
	deadlineClause := ""
	args := []any{userID, keyword}
	if cfg.ApplicationDays > 0 {
		deadlineClause = fmt.Sprintf(" AND COALESCE(po.completed_at, po.paid_at) + ($%d * INTERVAL '1 day') >= NOW()", len(args)+1)
		args = append(args, cfg.ApplicationDays)
	}
	base := `
		FROM payment_orders po
		WHERE po.user_id=$1
		  AND ($2='' OR po.out_trade_no ILIKE '%' || $2 || '%')
		  AND po.status='COMPLETED'
		  AND po.order_type IN ('balance','subscription','first_recharge_gift')
		  AND po.pay_amount > 0
		  AND po.refund_amount = 0
		  AND po.refund_requested_at IS NULL
		  AND po.refund_at IS NULL
		  AND COALESCE(po.completed_at, po.paid_at) IS NOT NULL
		  AND (` + invoicePaymentOrderCurrencySQL + `)='CNY'
		  AND NOT EXISTS (
			SELECT 1 FROM invoice_request_orders iro
			WHERE iro.payment_order_id=po.id AND iro.reservation_active IS TRUE
		  )` + deadlineClause
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+base, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	query := `SELECT po.id, po.out_trade_no, po.order_type, po.payment_type,
		po.pay_amount::text, COALESCE(po.completed_at, po.paid_at),
		COALESCE(po.completed_at, po.paid_at) + ($3 * INTERVAL '1 day'),
		po.paid_at, po.completed_at ` + base + `
		ORDER BY COALESCE(po.completed_at, po.paid_at) DESC, po.id DESC LIMIT $4 OFFSET $5`
	if cfg.ApplicationDays == 0 {
		query = `SELECT po.id, po.out_trade_no, po.order_type, po.payment_type,
			po.pay_amount::text, COALESCE(po.completed_at, po.paid_at), NULL::timestamptz,
			po.paid_at, po.completed_at ` + base + `
			ORDER BY COALESCE(po.completed_at, po.paid_at) DESC, po.id DESC LIMIT $3 OFFSET $4`
		listArgs = []any{userID, keyword, pageSize, (page - 1) * pageSize}
	}
	rows, err := r.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item service.InvoiceOrderSnapshot
		var anchor time.Time
		var deadline, paidAt, completedAt sql.NullTime
		if err := rows.Scan(&item.PaymentOrderID, &item.OutTradeNo, &item.OrderType, &item.PaymentType, &item.PayAmount, &anchor, &deadline, &paidAt, &completedAt); err != nil {
			return result, err
		}
		item.ID = item.PaymentOrderID
		item.Currency = payment.DefaultPaymentCurrency
		item.ReservationActive = false
		item.ApplicationAnchor = &anchor
		item.ApplicationDeadline = nullTimePtr(deadline)
		item.PaidAt = nullTimePtr(paidAt)
		item.CompletedAt = nullTimePtr(completedAt)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	var status, refunded, reserved, expired, currency int64
	reasonQuery := `SELECT
		COUNT(*) FILTER (WHERE status <> 'COMPLETED'),
		COUNT(*) FILTER (WHERE refund_amount <> 0 OR refund_requested_at IS NOT NULL OR refund_at IS NOT NULL OR status IN ('REFUND_REQUESTED','REFUNDING','REFUND_PENDING','PARTIALLY_REFUNDED','REFUNDED','REFUND_FAILED')),
		COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM invoice_request_orders iro WHERE iro.payment_order_id=po.id AND iro.reservation_active IS TRUE)),
		COUNT(*) FILTER (WHERE $2::int > 0 AND COALESCE(completed_at, paid_at) + ($2 * INTERVAL '1 day') < NOW()),
		COUNT(*) FILTER (WHERE (` + invoicePaymentOrderCurrencySQL + `) <> 'CNY')
		FROM payment_orders po WHERE user_id=$1 AND order_type IN ('balance','subscription','first_recharge_gift')`
	if err := r.db.QueryRowContext(ctx, reasonQuery, userID, cfg.ApplicationDays).Scan(&status, &refunded, &reserved, &expired, &currency); err == nil {
		result.IneligibleReasons["status"] = status
		result.IneligibleReasons["refund"] = refunded
		result.IneligibleReasons["reserved"] = reserved
		result.IneligibleReasons["expired"] = expired
		result.IneligibleReasons["currency"] = currency
	}
	return result, nil
}

func (r *invoiceRepository) Create(ctx context.Context, params service.InvoiceCreateParams) (*service.InvoiceRequest, error) {
	if r == nil || r.db == nil {
		return nil, infraerrors.InternalServer("INVOICE_CONFIG_INVALID", "invoice repository is unavailable")
	}
	ids, err := normalizeInvoiceOrderIDs(params.OrderIDs, params.Config.MaxOrdersPerRequest)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	var userEmail, userName string
	if err := tx.QueryRowContext(ctx, `SELECT email, username FROM users WHERE id=$1 AND deleted_at IS NULL`, params.UserID).Scan(&userEmail, &userName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("ORDER_NOT_FOUND", "selected orders were not found")
		}
		return nil, err
	}
	orders, err := loadLockedPaymentOrders(ctx, tx, params.UserID, ids)
	if err != nil {
		return nil, err
	}
	total, err := validateInvoiceOrders(orders, params.Config, time.Now())
	if err != nil {
		return nil, err
	}
	requestNo, err := newInvoiceRequestNo(time.Now())
	if err != nil {
		return nil, err
	}
	configRaw, err := json.Marshal(params.Config)
	if err != nil {
		return nil, err
	}
	var requestID int64
	var createdAt, updatedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO invoice_requests (
			request_no,user_id,user_email_snapshot,user_name_snapshot,status,title_type,title_name,
			taxpayer_id,recipient_email,recipient_phone,company_address,company_phone,bank_name,
			bank_account,invoice_item_name,currency,total_amount,order_count,user_note,config_snapshot
		) VALUES ($1,$2,$3,$4,'PENDING',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'CNY',$15,$16,$17,$18)
		RETURNING id,created_at,updated_at
	`, requestNo, params.UserID, userEmail, userName, params.Header.TitleType, params.Header.TitleName,
		params.Header.TaxpayerID, params.Header.RecipientEmail, params.Header.RecipientPhone,
		params.Header.CompanyAddress, params.Header.CompanyPhone, params.Header.BankName,
		params.Header.BankAccount, params.Config.ItemName, total.StringFixed(2), len(orders),
		params.Header.UserNote, configRaw).Scan(&requestID, &createdAt, &updatedAt)
	if err != nil {
		return nil, mapInvoiceWriteError(err)
	}
	for _, order := range orders {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO invoice_request_orders (
				invoice_request_id,payment_order_id,reservation_active,out_trade_no_snapshot,
				order_type_snapshot,payment_type_snapshot,pay_amount_snapshot,currency_snapshot,
				paid_at_snapshot,completed_at_snapshot
			) VALUES ($1,$2,TRUE,$3,$4,$5,$6,$7,$8,$9)
		`, requestID, order.ID, order.OutTradeNo, order.OrderType, order.PaymentType, order.PayAmount,
			paymentOrderCurrency(order.ProviderSnapshot), nullTimeValue(order.PaidAt), nullTimeValue(order.CompletedAt))
		if err != nil {
			return nil, mapInvoiceWriteError(err)
		}
	}
	if err := insertInvoiceAudit(ctx, tx, requestID, requestNo, "submit", "", service.InvoiceStatusPending, 1, params.Actor, map[string]any{"order_ids": ids}); err != nil {
		return nil, err
	}
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, mapInvoiceWriteError(err)
	}
	return r.GetUser(ctx, requestID, params.UserID)
}

func (r *invoiceRepository) ListUser(ctx context.Context, userID int64, page, pageSize int) ([]service.InvoiceRequest, int64, error) {
	page, pageSize = invoicePagination(page, pageSize)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoice_requests WHERE user_id=$1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+invoiceRequestColumns+` FROM invoice_requests WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanInvoiceRequests(rows)
	return items, total, err
}

func (r *invoiceRepository) GetUser(ctx context.Context, requestID, userID int64) (*service.InvoiceRequest, error) {
	request, err := scanInvoiceRequest(r.db.QueryRowContext(ctx, `SELECT `+invoiceRequestColumns+` FROM invoice_requests WHERE id=$1 AND user_id=$2`, requestID, userID))
	if err != nil {
		return nil, invoiceNotFound(err)
	}
	if err := r.loadRequestDetails(ctx, r.db, request); err != nil {
		return nil, err
	}
	return request, nil
}

func (r *invoiceRepository) Cancel(ctx context.Context, requestID, userID int64, actor service.InvoiceAuditActor) (*service.InvoiceRequest, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Keep the repository boundary fail-closed as well as the service boundary:
	// cancellation changes status and releases payment-order reservations.
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	request, err := lockInvoiceRequest(ctx, tx, requestID)
	if err != nil || request.UserID != userID {
		return nil, invoiceNotFound(err)
	}
	if request.Status == service.InvoiceStatusCancelled {
		_ = tx.Commit()
		return r.GetUser(ctx, requestID, userID)
	}
	if request.Status != service.InvoiceStatusPending && request.Status != service.InvoiceStatusRejected {
		return nil, invalidInvoiceTransition()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invoice_requests SET status='CANCELLED',cancelled_at=NOW(),updated_at=NOW() WHERE id=$1`, requestID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invoice_request_orders SET reservation_active=FALSE,released_at=NOW() WHERE invoice_request_id=$1 AND reservation_active IS TRUE`, requestID); err != nil {
		return nil, err
	}
	if err := insertInvoiceAudit(ctx, tx, requestID, request.RequestNo, "cancel", request.Status, service.InvoiceStatusCancelled, request.Revision, actor, nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetUser(ctx, requestID, userID)
}

func (r *invoiceRepository) Resubmit(ctx context.Context, params service.InvoiceResubmitParams) (*service.InvoiceRequest, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	request, err := lockInvoiceRequest(ctx, tx, params.RequestID)
	if err != nil || request.UserID != params.UserID {
		return nil, invoiceNotFound(err)
	}
	if request.Status != service.InvoiceStatusRejected {
		return nil, invalidInvoiceTransition()
	}
	ids, snapshots, err := loadInvoiceOrderReservations(ctx, tx, params.RequestID)
	if err != nil {
		return nil, err
	}
	orders, err := loadLockedPaymentOrders(ctx, tx, params.UserID, ids)
	if err != nil {
		return nil, err
	}
	total, err := validateInvoiceOrders(orders, params.Config, time.Now())
	if err != nil {
		return nil, err
	}
	if !total.Equal(mustDecimal(request.TotalAmount)) {
		return nil, infraerrors.Conflict("ORDER_NOT_ELIGIBLE", "selected order amount changed")
	}
	for _, order := range orders {
		snapshot, ok := snapshots[order.ID]
		if !ok || snapshot.PayAmount != mustDecimal(order.PayAmount).StringFixed(2) || snapshot.OrderType != order.OrderType || snapshot.Currency != paymentOrderCurrency(order.ProviderSnapshot) {
			return nil, infraerrors.Conflict("ORDER_NOT_ELIGIBLE", "selected order changed")
		}
	}
	configRaw, _ := json.Marshal(params.Config)
	newRevision := request.Revision + 1
	_, err = tx.ExecContext(ctx, `UPDATE invoice_requests SET status='PENDING',title_type=$2,title_name=$3,taxpayer_id=$4,
		recipient_email=$5,recipient_phone=$6,company_address=$7,company_phone=$8,bank_name=$9,bank_account=$10,
		user_note=$11,reject_reason='',rejected_by=NULL,rejected_at=NULL,accepted_by=NULL,accepted_at=NULL,
		config_snapshot=$12,revision=$13,updated_at=NOW() WHERE id=$1`, params.RequestID, params.Header.TitleType,
		params.Header.TitleName, params.Header.TaxpayerID, params.Header.RecipientEmail, params.Header.RecipientPhone,
		params.Header.CompanyAddress, params.Header.CompanyPhone, params.Header.BankName, params.Header.BankAccount,
		params.Header.UserNote, configRaw, newRevision)
	if err != nil {
		return nil, err
	}
	if err := insertInvoiceAudit(ctx, tx, request.ID, request.RequestNo, "resubmit", request.Status, service.InvoiceStatusPending, newRevision, params.Actor, nil); err != nil {
		return nil, err
	}
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetUser(ctx, params.RequestID, params.UserID)
}

func (r *invoiceRepository) ListAdmin(ctx context.Context, params service.InvoiceListParams) ([]service.InvoiceRequest, int64, error) {
	params.Page, params.PageSize = invoicePagination(params.Page, params.PageSize)
	where := []string{"1=1"}
	args := make([]any, 0, 10)
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if value := strings.TrimSpace(params.Status); value != "" {
		add("status=$%d", value)
	}
	if value := strings.TrimSpace(params.Keyword); value != "" {
		add("request_no ILIKE '%%' || $%d || '%%'", value)
	}
	if value := strings.TrimSpace(params.UserEmail); value != "" {
		add("user_email_snapshot ILIKE '%%' || $%d || '%%'", value)
	}
	if value := strings.TrimSpace(params.TitleName); value != "" {
		add("title_name ILIKE '%%' || $%d || '%%'", value)
	}
	if value := strings.TrimSpace(params.OrderNo); value != "" {
		add("EXISTS (SELECT 1 FROM invoice_request_orders iro WHERE iro.invoice_request_id=invoice_requests.id AND iro.out_trade_no_snapshot ILIKE '%%' || $%d || '%%')", value)
	}
	if params.CreatedFrom != nil {
		add("created_at >= $%d", *params.CreatedFrom)
	}
	if params.CreatedTo != nil {
		add("created_at <= $%d", *params.CreatedTo)
	}
	if params.IssuedFrom != nil {
		add("issued_at >= $%d", *params.IssuedFrom)
	}
	if params.IssuedTo != nil {
		add("issued_at <= $%d", *params.IssuedTo)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoice_requests WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	listArgs := append(append([]any{}, args...), params.PageSize, (params.Page-1)*params.PageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT `+invoiceRequestColumns+` FROM invoice_requests WHERE `+whereSQL+` ORDER BY created_at DESC,id DESC LIMIT $`+fmt.Sprint(len(args)+1)+` OFFSET $`+fmt.Sprint(len(args)+2), listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanInvoiceRequests(rows)
	return items, total, err
}

func (r *invoiceRepository) GetAdmin(ctx context.Context, requestID int64) (*service.InvoiceRequest, error) {
	request, err := scanInvoiceRequest(r.db.QueryRowContext(ctx, `SELECT `+invoiceRequestColumns+` FROM invoice_requests WHERE id=$1`, requestID))
	if err != nil {
		return nil, invoiceNotFound(err)
	}
	if err := r.loadRequestDetails(ctx, r.db, request); err != nil {
		return nil, err
	}
	return request, nil
}

func (r *invoiceRepository) Accept(ctx context.Context, params service.InvoiceAdminActionParams) (*service.InvoiceRequest, error) {
	return r.adminTransition(ctx, params, service.InvoiceStatusPending, service.InvoiceStatusProcessing, "accept", `accepted_by=$2,accepted_at=NOW()`)
}

func (r *invoiceRepository) Release(ctx context.Context, params service.InvoiceAdminActionParams) (*service.InvoiceRequest, error) {
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Reason == "" || len([]rune(params.Reason)) > 1000 {
		return nil, infraerrors.BadRequest("INVALID_INVOICE_STATUS_TRANSITION", "release reason is required")
	}
	return r.adminTransition(ctx, params, service.InvoiceStatusProcessing, service.InvoiceStatusPending, "release", `accepted_by=NULL,accepted_at=NULL`)
}

func (r *invoiceRepository) Reject(ctx context.Context, params service.InvoiceAdminActionParams) (*service.InvoiceRequest, error) {
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Reason == "" || len([]rune(params.Reason)) > 1000 {
		return nil, infraerrors.BadRequest("INVALID_INVOICE_STATUS_TRANSITION", "reject reason is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Re-check the independent rollout gate while holding the write
	// transaction. The service layer performs the same check for latency, but
	// an administrator can disable the feature between that check and this
	// transaction.
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	request, err := lockInvoiceRequest(ctx, tx, params.RequestID)
	if err != nil {
		return nil, invoiceNotFound(err)
	}
	if request.Status != service.InvoiceStatusPending && request.Status != service.InvoiceStatusProcessing {
		return nil, invalidInvoiceTransition()
	}
	_, err = tx.ExecContext(ctx, `UPDATE invoice_requests SET status='REJECTED',reject_reason=$2,rejected_by=$3,rejected_at=NOW(),updated_at=NOW() WHERE id=$1`, request.ID, params.Reason, params.AdminID)
	if err != nil {
		return nil, err
	}
	if err := insertInvoiceAudit(ctx, tx, request.ID, request.RequestNo, "reject", request.Status, service.InvoiceStatusRejected, request.Revision, params.Actor, map[string]any{"reason": params.Reason}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAdmin(ctx, request.ID)
}

func (r *invoiceRepository) adminTransition(ctx context.Context, params service.InvoiceAdminActionParams, from, to, action, setClause string) (*service.InvoiceRequest, error) {
	adminNote := strings.TrimSpace(firstNonEmptyInvoice(params.Note, params.Reason))
	if len([]rune(adminNote)) > 1000 {
		return nil, infraerrors.BadRequest("INVALID_INVOICE_STATUS_TRANSITION", "admin note is too long")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	request, err := lockInvoiceRequest(ctx, tx, params.RequestID)
	if err != nil {
		return nil, invoiceNotFound(err)
	}
	if request.Status != from {
		return nil, invalidInvoiceTransition()
	}
	query := `UPDATE invoice_requests SET status=$3,` + setClause + `,admin_note=$4,updated_at=NOW() WHERE id=$1 AND status=$5`
	result, err := tx.ExecContext(ctx, query, request.ID, params.AdminID, to, adminNote, from)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, invalidInvoiceTransition()
	}
	metadata := map[string]any{}
	if strings.TrimSpace(params.Reason) != "" {
		metadata["reason"] = strings.TrimSpace(params.Reason)
	}
	if err := insertInvoiceAudit(ctx, tx, request.ID, request.RequestNo, action, from, to, request.Revision, params.Actor, metadata); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAdmin(ctx, request.ID)
}

func (r *invoiceRepository) Issue(ctx context.Context, params service.InvoiceIssueParams) (*service.InvoiceRequest, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	request, err := lockInvoiceRequest(ctx, tx, params.RequestID)
	if err != nil {
		return nil, invoiceNotFound(err)
	}
	if request.Status != service.InvoiceStatusProcessing {
		if request.Status == service.InvoiceStatusIssued {
			var currentSHA string
			err = tx.QueryRowContext(ctx, `SELECT sha256 FROM invoice_files WHERE invoice_request_id=$1 AND is_current IS TRUE FOR SHARE`, request.ID).Scan(&currentSHA)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if err == nil && invoiceIssueReplayMatches(request, params, currentSHA) {
				if err := tx.Commit(); err != nil {
					return nil, err
				}
				return r.GetAdmin(ctx, request.ID)
			}
		}
		return nil, invalidInvoiceTransition()
	}
	ids, snapshots, err := loadInvoiceOrderReservations(ctx, tx, request.ID)
	if err != nil {
		return nil, err
	}
	orders, err := loadLockedPaymentOrders(ctx, tx, request.UserID, ids)
	if err != nil {
		return nil, err
	}
	issueConfig := request.ConfigSnapshot
	issueConfig.ApplicationDays = 0 // Accepted applications do not expire while being processed.
	if _, err := validateInvoiceOrders(orders, issueConfig, time.Now()); err != nil {
		return nil, err
	}
	if len(orders) != len(snapshots) {
		return nil, infraerrors.Conflict("ORDER_NOT_ELIGIBLE", "selected order is missing")
	}
	for _, order := range orders {
		snapshot, ok := snapshots[order.ID]
		if !ok || !snapshot.ReservationActive || snapshot.PayAmount != mustDecimal(order.PayAmount).StringFixed(2) || snapshot.Currency != paymentOrderCurrency(order.ProviderSnapshot) || snapshot.OrderType != order.OrderType {
			return nil, infraerrors.Conflict("ORDER_NOT_ELIGIBLE", "selected order changed")
		}
	}
	if err := insertInvoiceFile(ctx, tx, request.ID, params.File); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE invoice_requests SET status='ISSUED',invoice_code=$2,invoice_number=$3,invoice_date=$4,issued_by=$5,issued_at=NOW(),updated_at=NOW() WHERE id=$1 AND status='PROCESSING'`, request.ID, params.InvoiceCode, params.InvoiceNumber, params.InvoiceDate, params.AdminID)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, invalidInvoiceTransition()
	}
	if err := insertInvoiceAudit(ctx, tx, request.ID, request.RequestNo, "issue", request.Status, service.InvoiceStatusIssued, request.Revision, params.Actor, map[string]any{"file_sha256": params.File.SHA256, "invoice_date": params.InvoiceDate.Format("2006-01-02")}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAdmin(ctx, request.ID)
}

func invoiceIssueReplayMatches(request *service.InvoiceRequest, params service.InvoiceIssueParams, currentSHA string) bool {
	return request != nil && request.Status == service.InvoiceStatusIssued && request.InvoiceDate != nil &&
		request.InvoiceCode == params.InvoiceCode && request.InvoiceNumber == params.InvoiceNumber &&
		request.InvoiceDate.Format("2006-01-02") == params.InvoiceDate.Format("2006-01-02") &&
		currentSHA != "" && currentSHA == params.File.SHA256
}

func (r *invoiceRepository) ReplaceFile(ctx context.Context, params service.InvoiceReplaceFileParams) (*service.InvoiceRequest, error) {
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Reason == "" || len([]rune(params.Reason)) > 1000 {
		return nil, infraerrors.BadRequest("INVALID_INVOICE_STATUS_TRANSITION", "replacement reason is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	request, err := lockInvoiceRequest(ctx, tx, params.RequestID)
	if err != nil {
		return nil, invoiceNotFound(err)
	}
	if request.Status != service.InvoiceStatusIssued {
		return nil, invalidInvoiceTransition()
	}
	var currentSHA string
	if err := tx.QueryRowContext(ctx, `SELECT sha256 FROM invoice_files WHERE invoice_request_id=$1 AND is_current IS TRUE FOR UPDATE`, request.ID).Scan(&currentSHA); err != nil {
		return nil, infraerrors.Conflict("INVOICE_FILE_NOT_FOUND", "current invoice file was not found").WithCause(err)
	}
	if currentSHA == params.File.SHA256 {
		_ = tx.Commit()
		return r.GetAdmin(ctx, request.ID)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invoice_files SET is_current=FALSE,replaced_at=NOW() WHERE invoice_request_id=$1 AND is_current IS TRUE`, request.ID); err != nil {
		return nil, err
	}
	if err := insertInvoiceFile(ctx, tx, request.ID, params.File); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invoice_requests SET invoice_date=$2,updated_at=NOW() WHERE id=$1`, request.ID, params.InvoiceDate); err != nil {
		return nil, err
	}
	if err := insertInvoiceAudit(ctx, tx, request.ID, request.RequestNo, "replace_file", request.Status, request.Status, request.Revision, params.Actor, map[string]any{"reason": params.Reason, "old_file_sha256": currentSHA, "new_file_sha256": params.File.SHA256, "invoice_date": params.InvoiceDate.Format("2006-01-02")}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAdmin(ctx, request.ID)
}

func (r *invoiceRepository) Void(ctx context.Context, params service.InvoiceAdminActionParams) (*service.InvoiceRequest, error) {
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Reason == "" || len([]rune(params.Reason)) > 1000 {
		return nil, infraerrors.BadRequest("INVALID_INVOICE_STATUS_TRANSITION", "void reason is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensureInvoiceEnabledInTx(ctx, tx); err != nil {
		return nil, err
	}
	request, err := lockInvoiceRequest(ctx, tx, params.RequestID)
	if err != nil {
		return nil, invoiceNotFound(err)
	}
	if request.Status != service.InvoiceStatusIssued {
		return nil, invalidInvoiceTransition()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invoice_requests SET status='VOIDED',voided_by=$2,voided_at=NOW(),admin_note=$3,updated_at=NOW() WHERE id=$1`, request.ID, params.AdminID, params.Reason); err != nil {
		return nil, err
	}
	if request.ConfigSnapshot.AllowReapplyAfterVoid {
		if _, err := tx.ExecContext(ctx, `UPDATE invoice_request_orders SET reservation_active=FALSE,released_at=NOW() WHERE invoice_request_id=$1 AND reservation_active IS TRUE`, request.ID); err != nil {
			return nil, err
		}
	}
	if err := insertInvoiceAudit(ctx, tx, request.ID, request.RequestNo, "void", request.Status, service.InvoiceStatusVoided, request.Revision, params.Actor, map[string]any{"reason": params.Reason, "orders_released": request.ConfigSnapshot.AllowReapplyAfterVoid}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAdmin(ctx, request.ID)
}

func (r *invoiceRepository) ListAuditLogs(ctx context.Context, requestID int64) ([]service.InvoiceAuditLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,invoice_request_id,request_no_snapshot,actor_type,actor_id,action,from_status,to_status,request_revision,metadata::text,ip_address,created_at FROM invoice_audit_logs WHERE invoice_request_id=$1 ORDER BY created_at ASC,id ASC`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []service.InvoiceAuditLog{}
	for rows.Next() {
		var item service.InvoiceAuditLog
		var actorID sql.NullInt64
		var metadata string
		if err := rows.Scan(&item.ID, &item.InvoiceRequestID, &item.RequestNo, &item.ActorType, &actorID, &item.Action, &item.FromStatus, &item.ToStatus, &item.RequestRevision, &metadata, &item.IPAddress, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ActorID = invoiceNullInt64Ptr(actorID)
		_ = json.Unmarshal([]byte(metadata), &item.Metadata)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *invoiceRepository) GetCurrentFileForUser(ctx context.Context, requestID, userID int64) (*service.InvoiceRequest, *service.InvoiceFileMetadata, error) {
	request, err := r.GetUser(ctx, requestID, userID)
	if err != nil {
		return nil, nil, err
	}
	if request.Status != service.InvoiceStatusIssued && request.Status != service.InvoiceStatusVoided {
		return nil, nil, infraerrors.NotFound("INVOICE_FILE_NOT_FOUND", "invoice file was not found")
	}
	if request.CurrentFile == nil {
		return nil, nil, infraerrors.NotFound("INVOICE_FILE_NOT_FOUND", "invoice file was not found")
	}
	return request, request.CurrentFile, nil
}

func (r *invoiceRepository) GetCurrentFileForAdmin(ctx context.Context, requestID int64) (*service.InvoiceRequest, *service.InvoiceFileMetadata, error) {
	request, err := r.GetAdmin(ctx, requestID)
	if err != nil {
		return nil, nil, err
	}
	if request.CurrentFile == nil {
		return nil, nil, infraerrors.NotFound("INVOICE_FILE_NOT_FOUND", "invoice file was not found")
	}
	return request, request.CurrentFile, nil
}

func (r *invoiceRepository) ListAllFiles(ctx context.Context) ([]service.InvoiceFileMetadata, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,invoice_request_id,storage_key,original_filename,content_type,file_extension,file_size,sha256,is_current,uploaded_by,uploaded_at,replaced_at FROM invoice_files ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]service.InvoiceFileMetadata, 0)
	for rows.Next() {
		item, err := scanInvoiceFile(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *invoiceRepository) loadRequestDetails(ctx context.Context, q invoiceQueryer, request *service.InvoiceRequest) error {
	rows, err := q.QueryContext(ctx, `SELECT id,invoice_request_id,payment_order_id,out_trade_no_snapshot,order_type_snapshot,payment_type_snapshot,pay_amount_snapshot::text,currency_snapshot,paid_at_snapshot,completed_at_snapshot,reservation_active,released_at FROM invoice_request_orders WHERE invoice_request_id=$1 ORDER BY id`, request.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item service.InvoiceOrderSnapshot
		var paid, completed, released sql.NullTime
		if err := rows.Scan(&item.ID, &item.InvoiceRequestID, &item.PaymentOrderID, &item.OutTradeNo, &item.OrderType, &item.PaymentType, &item.PayAmount, &item.Currency, &paid, &completed, &item.ReservationActive, &released); err != nil {
			return err
		}
		item.PaidAt, item.CompletedAt, item.ReleasedAt = nullTimePtr(paid), nullTimePtr(completed), nullTimePtr(released)
		request.Orders = append(request.Orders, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	file, err := scanInvoiceFile(q.QueryRowContext(ctx, `SELECT id,invoice_request_id,storage_key,original_filename,content_type,file_extension,file_size,sha256,is_current,uploaded_by,uploaded_at,replaced_at FROM invoice_files WHERE invoice_request_id=$1 AND is_current IS TRUE`, request.ID))
	if err == nil {
		request.CurrentFile = file
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func scanInvoiceRequests(rows *sql.Rows) ([]service.InvoiceRequest, error) {
	items := []service.InvoiceRequest{}
	for rows.Next() {
		item, err := scanInvoiceRequest(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func scanInvoiceRequest(scanner invoiceScanner) (*service.InvoiceRequest, error) {
	var item service.InvoiceRequest
	var configRaw string
	var invoiceDate, acceptedAt, issuedAt, rejectedAt, cancelledAt, voidedAt sql.NullTime
	var acceptedBy, issuedBy, rejectedBy, voidedBy sql.NullInt64
	err := scanner.Scan(&item.ID, &item.RequestNo, &item.UserID, &item.UserEmail, &item.UserName, &item.Status,
		&item.TitleType, &item.TitleName, &item.TaxpayerID, &item.RecipientEmail, &item.RecipientPhone,
		&item.CompanyAddress, &item.CompanyPhone, &item.BankName, &item.BankAccount, &item.InvoiceItemName,
		&item.Currency, &item.TotalAmount, &item.OrderCount, &item.UserNote, &item.AdminNote, &item.RejectReason,
		&item.InvoiceCode, &item.InvoiceNumber, &invoiceDate, &configRaw, &item.Revision, &acceptedBy, &acceptedAt,
		&issuedBy, &issuedAt, &rejectedBy, &rejectedAt, &cancelledAt, &voidedBy, &voidedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	item.InvoiceDate, item.AcceptedAt, item.IssuedAt, item.RejectedAt = nullTimePtr(invoiceDate), nullTimePtr(acceptedAt), nullTimePtr(issuedAt), nullTimePtr(rejectedAt)
	item.CancelledAt, item.VoidedAt = nullTimePtr(cancelledAt), nullTimePtr(voidedAt)
	item.AcceptedBy, item.IssuedBy, item.RejectedBy, item.VoidedBy = invoiceNullInt64Ptr(acceptedBy), invoiceNullInt64Ptr(issuedBy), invoiceNullInt64Ptr(rejectedBy), invoiceNullInt64Ptr(voidedBy)
	if err := json.Unmarshal([]byte(configRaw), &item.ConfigSnapshot); err != nil {
		return nil, fmt.Errorf("decode invoice config snapshot: %w", err)
	}
	return &item, nil
}

func scanInvoiceFile(scanner invoiceScanner) (*service.InvoiceFileMetadata, error) {
	var item service.InvoiceFileMetadata
	var replaced sql.NullTime
	if err := scanner.Scan(&item.ID, &item.InvoiceRequestID, &item.StorageKey, &item.OriginalFilename, &item.ContentType, &item.FileExtension, &item.FileSize, &item.SHA256, &item.IsCurrent, &item.UploadedBy, &item.UploadedAt, &replaced); err != nil {
		return nil, err
	}
	item.ReplacedAt = nullTimePtr(replaced)
	return &item, nil
}

func lockInvoiceRequest(ctx context.Context, tx *sql.Tx, requestID int64) (*service.InvoiceRequest, error) {
	return scanInvoiceRequest(tx.QueryRowContext(ctx, `SELECT `+invoiceRequestColumns+` FROM invoice_requests WHERE id=$1 FOR UPDATE`, requestID))
}

func loadLockedPaymentOrders(ctx context.Context, tx *sql.Tx, userID int64, ids []int64) ([]lockedPaymentOrder, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,out_trade_no,order_type,payment_type,pay_amount::text,provider_snapshot::text,status,refund_amount::text,refund_requested_at,refund_at,paid_at,completed_at FROM payment_orders WHERE user_id=$1 AND id=ANY($2) ORDER BY id FOR SHARE`, userID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := make([]lockedPaymentOrder, 0, len(ids))
	for rows.Next() {
		var item lockedPaymentOrder
		if err := rows.Scan(&item.ID, &item.OutTradeNo, &item.OrderType, &item.PaymentType, &item.PayAmount, &item.ProviderSnapshot, &item.Status, &item.RefundAmount, &item.RefundRequestedAt, &item.RefundAt, &item.PaidAt, &item.CompletedAt); err != nil {
			return nil, err
		}
		orders = append(orders, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(orders) != len(ids) {
		return nil, infraerrors.NotFound("ORDER_NOT_FOUND", "selected orders were not found")
	}
	for i, id := range ids {
		if orders[i].ID != id {
			return nil, infraerrors.NotFound("ORDER_NOT_FOUND", "selected orders were not found")
		}
	}
	return orders, nil
}

func validateInvoiceOrders(orders []lockedPaymentOrder, cfg service.InvoiceConfig, now time.Time) (decimal.Decimal, error) {
	total := decimal.Zero
	for _, order := range orders {
		if order.Status != payment.OrderStatusCompleted || isInvoiceRefundStatus(order.Status) || mustDecimal(order.RefundAmount).Cmp(decimal.Zero) != 0 || order.RefundRequestedAt.Valid || order.RefundAt.Valid {
			return decimal.Zero, infraerrors.Conflict("ORDER_NOT_ELIGIBLE", "selected order is not eligible")
		}
		switch order.OrderType {
		case payment.OrderTypeBalance, payment.OrderTypeSubscription, payment.OrderTypeFirstRechargeGift:
		default:
			return decimal.Zero, infraerrors.Conflict("ORDER_NOT_ELIGIBLE", "selected order type is not eligible")
		}
		amount, err := decimal.NewFromString(order.PayAmount)
		if err != nil || amount.Cmp(decimal.Zero) <= 0 {
			return decimal.Zero, infraerrors.Conflict("ORDER_NOT_ELIGIBLE", "selected order amount is invalid")
		}
		if paymentOrderCurrency(order.ProviderSnapshot) != payment.DefaultPaymentCurrency {
			return decimal.Zero, infraerrors.BadRequest("ORDER_CURRENCY_UNSUPPORTED", "selected order currency is unsupported")
		}
		anchor := order.PaidAt
		if order.CompletedAt.Valid {
			anchor = order.CompletedAt
		}
		if !anchor.Valid {
			return decimal.Zero, infraerrors.Conflict("ORDER_NOT_ELIGIBLE", "selected order completion time is missing")
		}
		if cfg.ApplicationDays > 0 && now.After(anchor.Time.Add(time.Duration(cfg.ApplicationDays)*24*time.Hour)) {
			return decimal.Zero, infraerrors.Conflict("INVOICE_APPLICATION_EXPIRED", "selected order is outside the invoice application period")
		}
		total = total.Add(amount)
	}
	min, err := invoiceConfigAmountDecimal(cfg.MinAmount)
	if err != nil || min.Cmp(decimal.Zero) <= 0 {
		return decimal.Zero, infraerrors.BadRequest("INVOICE_CONFIG_INVALID", "invoice minimum amount is invalid")
	}
	max, err := invoiceConfigAmountDecimal(cfg.MaxAmount)
	if err != nil || max.Cmp(decimal.Zero) < 0 || (max.Cmp(decimal.Zero) > 0 && max.Cmp(min) < 0) {
		return decimal.Zero, infraerrors.BadRequest("INVOICE_CONFIG_INVALID", "invoice maximum amount is invalid")
	}
	if total.Cmp(min) < 0 {
		return decimal.Zero, infraerrors.BadRequest("INVOICE_AMOUNT_BELOW_MINIMUM", "invoice amount is below the configured minimum")
	}
	if max.Cmp(decimal.Zero) > 0 && total.Cmp(max) > 0 {
		return decimal.Zero, infraerrors.BadRequest("INVOICE_AMOUNT_ABOVE_MAXIMUM", "invoice amount is above the configured maximum")
	}
	return total, nil
}

func invoiceConfigAmountDecimal(value float64) (decimal.Decimal, error) {
	return decimal.NewFromString(strconv.FormatFloat(value, 'f', 2, 64))
}

func loadInvoiceOrderReservations(ctx context.Context, tx *sql.Tx, requestID int64) ([]int64, map[int64]service.InvoiceOrderSnapshot, error) {
	rows, err := tx.QueryContext(ctx, `SELECT payment_order_id,pay_amount_snapshot::text,order_type_snapshot,currency_snapshot,reservation_active FROM invoice_request_orders WHERE invoice_request_id=$1 ORDER BY payment_order_id FOR SHARE`, requestID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	ids := []int64{}
	snapshots := map[int64]service.InvoiceOrderSnapshot{}
	for rows.Next() {
		var item service.InvoiceOrderSnapshot
		if err := rows.Scan(&item.PaymentOrderID, &item.PayAmount, &item.OrderType, &item.Currency, &item.ReservationActive); err != nil {
			return nil, nil, err
		}
		ids = append(ids, item.PaymentOrderID)
		snapshots[item.PaymentOrderID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		return nil, nil, infraerrors.Conflict("ORDER_NOT_ELIGIBLE", "invoice order snapshot is missing")
	}
	return ids, snapshots, nil
}

func ensureInvoiceEnabledInTx(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return infraerrors.Forbidden("INVOICE_DISABLED", "invoice applications are disabled")
	}
	// Read both settings under the same transaction. FOR SHARE makes a
	// concurrent settings update wait until this mutation commits, while a
	// missing namespaced row remains fail-closed (and therefore cannot be
	// accidentally enabled by a legacy JSON value).
	rows, err := tx.QueryContext(ctx, `
		SELECT key, value
		FROM settings
		WHERE key IN ($1, $2)
		FOR SHARE
	`, service.SettingKeyInvoiceConfig, service.SettingKeySubNexusInvoiceEnabled)
	if err != nil {
		return err
	}
	values := make(map[string]string, 2)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			_ = rows.Close()
			return err
		}
		values[key] = value
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if rows.Err() != nil {
		return rows.Err()
	}
	if values[service.SettingKeySubNexusInvoiceEnabled] != "true" {
		return infraerrors.Forbidden("INVOICE_DISABLED", "invoice applications are disabled")
	}
	raw, ok := values[service.SettingKeyInvoiceConfig]
	if !ok || strings.TrimSpace(raw) == "" {
		return infraerrors.Forbidden("INVOICE_DISABLED", "invoice applications are disabled")
	}
	var cfg service.InvoiceConfig
	if json.Unmarshal([]byte(raw), &cfg) != nil || !cfg.Enabled {
		return infraerrors.Forbidden("INVOICE_DISABLED", "invoice applications are disabled")
	}
	return nil
}

func insertInvoiceAudit(ctx context.Context, tx *sql.Tx, requestID int64, requestNo, action, from, to string, revision int, actor service.InvoiceAuditActor, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO invoice_audit_logs(invoice_request_id,request_no_snapshot,actor_type,actor_id,action,from_status,to_status,request_revision,metadata,ip_address,user_agent_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, requestID, requestNo, actor.Type, actor.ID, action, from, to, revision, raw, actor.IPAddress, actor.UserAgentHash)
	return err
}

func insertInvoiceFile(ctx context.Context, tx *sql.Tx, requestID int64, file service.InvoiceFileMetadata) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO invoice_files(invoice_request_id,storage_key,original_filename,content_type,file_extension,file_size,sha256,is_current,uploaded_by) VALUES($1,$2,$3,$4,$5,$6,$7,TRUE,$8)`, requestID, file.StorageKey, file.OriginalFilename, file.ContentType, file.FileExtension, file.FileSize, file.SHA256, file.UploadedBy)
	return mapInvoiceWriteError(err)
}

func normalizeInvoiceOrderIDs(ids []int64, max int) ([]int64, error) {
	if len(ids) == 0 || len(ids) > max || len(ids) > 100 {
		return nil, infraerrors.BadRequest("INVALID_ORDER_SELECTION", "select between 1 and the configured maximum number of orders")
	}
	set := map[int64]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			return nil, infraerrors.BadRequest("INVALID_ORDER_SELECTION", "selected order id is invalid")
		}
		set[id] = struct{}{}
	}
	if len(set) != len(ids) {
		return nil, infraerrors.BadRequest("INVALID_ORDER_SELECTION", "duplicate order ids are not allowed")
	}
	out := append([]int64(nil), ids...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func newInvoiceRequestNo(now time.Time) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "INV-" + now.UTC().Format("20060102") + "-" + strings.ToUpper(hex.EncodeToString(b)), nil
}

func paymentOrderCurrency(raw string) string {
	var snapshot map[string]any
	if json.Unmarshal([]byte(raw), &snapshot) != nil {
		return ""
	}
	value, exists := snapshot["currency"]
	if !exists || value == nil {
		return payment.DefaultPaymentCurrency
	}
	rawCurrency, ok := value.(string)
	if !ok {
		return ""
	}
	if strings.TrimSpace(rawCurrency) == "" {
		return payment.DefaultPaymentCurrency
	}
	currency, err := payment.NormalizePaymentCurrency(rawCurrency)
	if err != nil {
		return ""
	}
	return currency
}
func isInvoiceRefundStatus(status string) bool {
	switch status {
	case payment.OrderStatusRefundRequested, payment.OrderStatusRefunding, payment.OrderStatusRefundPending, payment.OrderStatusPartiallyRefunded, payment.OrderStatusRefunded, payment.OrderStatusRefundFailed:
		return true
	}
	return false
}
func invoicePagination(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	value := v.Time
	return &value
}
func invoiceNullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}
func nullTimeValue(v sql.NullTime) any {
	if !v.Valid {
		return nil
	}
	return v.Time
}
func mustDecimal(value string) decimal.Decimal {
	result, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero
	}
	return result
}
func invoiceNotFound(err error) error {
	if err == nil {
		return infraerrors.NotFound("INVOICE_NOT_FOUND", "invoice request was not found")
	}
	if errors.Is(err, sql.ErrNoRows) {
		return infraerrors.NotFound("INVOICE_NOT_FOUND", "invoice request was not found")
	}
	return err
}
func invalidInvoiceTransition() error {
	return infraerrors.Conflict("INVALID_INVOICE_STATUS_TRANSITION", "invoice request status changed")
}
func mapInvoiceWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		if strings.Contains(pqErr.Constraint, "active_payment") {
			return infraerrors.Conflict("ORDER_ALREADY_RESERVED", "one or more selected orders are already reserved")
		}
		return infraerrors.Conflict("INVALID_INVOICE_STATUS_TRANSITION", "invoice data already exists")
	}
	return err
}
func firstNonEmptyInvoice(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
