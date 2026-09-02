package service

import (
	"context"
	"io"
	"time"
)

const (
	// The namespaced rollout switch is separate from the legacy JSON payload.
	// A shared database may contain INVOICE_CONFIG.enabled=true from the old
	// application; that value alone must never enable the migrated workflow.
	SettingKeySubNexusInvoiceEnabled = "subnexus_invoice_enabled"
	SettingKeyInvoiceConfig          = "INVOICE_CONFIG"
	SettingKeyInvoiceConfigAudit     = "INVOICE_CONFIG_AUDIT:"

	InvoiceStatusPending    = "PENDING"
	InvoiceStatusProcessing = "PROCESSING"
	InvoiceStatusRejected   = "REJECTED"
	InvoiceStatusCancelled  = "CANCELLED"
	InvoiceStatusIssued     = "ISSUED"
	InvoiceStatusVoided     = "VOIDED"

	InvoiceTitlePersonal = "PERSONAL"
	InvoiceTitleCompany  = "COMPANY"
)

type InvoiceConfig struct {
	Enabled                 bool     `json:"enabled"`
	MinAmount               float64  `json:"min_amount"`
	MaxAmount               float64  `json:"max_amount"`
	ApplicationDays         int      `json:"application_days"`
	MaxOrdersPerRequest     int      `json:"max_orders_per_request"`
	ItemName                string   `json:"item_name"`
	AdminNotificationEmails []string `json:"admin_notification_emails"`
	MaxFileSizeMB           int      `json:"max_file_size_mb"`
	AllowReapplyAfterVoid   bool     `json:"allow_reapply_after_void"`
}

type InvoicePublicConfig struct {
	InvoiceConfig
	HasHistory        bool     `json:"has_history"`
	AllowedOrderTypes []string `json:"allowed_order_types"`
}

type InvoiceHeaderInput struct {
	TitleType      string `json:"title_type"`
	TitleName      string `json:"title_name"`
	TaxpayerID     string `json:"taxpayer_id"`
	RecipientEmail string `json:"recipient_email"`
	RecipientPhone string `json:"recipient_phone"`
	CompanyAddress string `json:"company_address"`
	CompanyPhone   string `json:"company_phone"`
	BankName       string `json:"bank_name"`
	BankAccount    string `json:"bank_account"`
	UserNote       string `json:"user_note"`
}

type InvoiceCreateInput struct {
	OrderIDs []int64 `json:"order_ids"`
	InvoiceHeaderInput
}

type InvoiceResubmitInput struct {
	InvoiceHeaderInput
}

type InvoiceOrderSnapshot struct {
	ID                  int64      `json:"id"`
	InvoiceRequestID    int64      `json:"invoice_request_id,omitempty"`
	PaymentOrderID      int64      `json:"payment_order_id"`
	OutTradeNo          string     `json:"out_trade_no"`
	OrderType           string     `json:"order_type"`
	PaymentType         string     `json:"payment_type"`
	PayAmount           string     `json:"pay_amount"`
	Currency            string     `json:"currency"`
	PaidAt              *time.Time `json:"paid_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	ApplicationAnchor   *time.Time `json:"application_anchor,omitempty"`
	ApplicationDeadline *time.Time `json:"application_deadline,omitempty"`
	ReservationActive   bool       `json:"reservation_active"`
	ReleasedAt          *time.Time `json:"released_at,omitempty"`
}

type InvoiceFileMetadata struct {
	ID               int64      `json:"id"`
	InvoiceRequestID int64      `json:"invoice_request_id"`
	StorageKey       string     `json:"-"`
	OriginalFilename string     `json:"original_filename"`
	ContentType      string     `json:"content_type"`
	FileExtension    string     `json:"file_extension"`
	FileSize         int64      `json:"file_size"`
	SHA256           string     `json:"sha256"`
	IsCurrent        bool       `json:"is_current"`
	UploadedBy       int64      `json:"uploaded_by,omitempty"`
	UploadedAt       time.Time  `json:"uploaded_at"`
	ReplacedAt       *time.Time `json:"replaced_at,omitempty"`
}

type InvoiceRequest struct {
	ID              int64                  `json:"id"`
	RequestNo       string                 `json:"request_no"`
	UserID          int64                  `json:"user_id"`
	UserEmail       string                 `json:"user_email"`
	UserName        string                 `json:"user_name"`
	Status          string                 `json:"status"`
	TitleType       string                 `json:"title_type"`
	TitleName       string                 `json:"title_name"`
	TaxpayerID      string                 `json:"taxpayer_id"`
	RecipientEmail  string                 `json:"recipient_email"`
	RecipientPhone  string                 `json:"recipient_phone"`
	CompanyAddress  string                 `json:"company_address"`
	CompanyPhone    string                 `json:"company_phone"`
	BankName        string                 `json:"bank_name"`
	BankAccount     string                 `json:"bank_account"`
	InvoiceItemName string                 `json:"invoice_item_name"`
	Currency        string                 `json:"currency"`
	TotalAmount     string                 `json:"total_amount"`
	OrderCount      int                    `json:"order_count"`
	UserNote        string                 `json:"user_note"`
	AdminNote       string                 `json:"admin_note"`
	RejectReason    string                 `json:"reject_reason"`
	InvoiceCode     string                 `json:"invoice_code"`
	InvoiceNumber   string                 `json:"invoice_number"`
	InvoiceDate     *time.Time             `json:"invoice_date,omitempty"`
	ConfigSnapshot  InvoiceConfig          `json:"config_snapshot"`
	Revision        int                    `json:"revision"`
	AcceptedBy      *int64                 `json:"accepted_by,omitempty"`
	AcceptedAt      *time.Time             `json:"accepted_at,omitempty"`
	IssuedBy        *int64                 `json:"issued_by,omitempty"`
	IssuedAt        *time.Time             `json:"issued_at,omitempty"`
	RejectedBy      *int64                 `json:"rejected_by,omitempty"`
	RejectedAt      *time.Time             `json:"rejected_at,omitempty"`
	CancelledAt     *time.Time             `json:"cancelled_at,omitempty"`
	VoidedBy        *int64                 `json:"voided_by,omitempty"`
	VoidedAt        *time.Time             `json:"voided_at,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	Orders          []InvoiceOrderSnapshot `json:"orders,omitempty"`
	CurrentFile     *InvoiceFileMetadata   `json:"current_file,omitempty"`
}

type InvoiceAuditLog struct {
	ID               int64          `json:"id"`
	InvoiceRequestID int64          `json:"invoice_request_id"`
	RequestNo        string         `json:"request_no"`
	ActorType        string         `json:"actor_type"`
	ActorID          *int64         `json:"actor_id,omitempty"`
	Action           string         `json:"action"`
	FromStatus       string         `json:"from_status"`
	ToStatus         string         `json:"to_status"`
	RequestRevision  int            `json:"request_revision"`
	Metadata         map[string]any `json:"metadata"`
	IPAddress        string         `json:"ip_address"`
	CreatedAt        time.Time      `json:"created_at"`
}

type InvoiceListParams struct {
	Page        int
	PageSize    int
	Status      string
	Keyword     string
	UserEmail   string
	TitleName   string
	OrderNo     string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	IssuedFrom  *time.Time
	IssuedTo    *time.Time
}

type InvoiceAuditActor struct {
	Type          string
	ID            *int64
	IPAddress     string
	UserAgentHash string
}

type InvoiceStorageStatus struct {
	Available     bool      `json:"available"`
	FreeBytes     uint64    `json:"free_bytes"`
	CheckedAt     time.Time `json:"checked_at"`
	FailureReason string    `json:"failure_reason,omitempty"`
}

type InvoiceAdminConfigResult struct {
	Config       InvoiceConfig             `json:"config"`
	Storage      InvoiceStorageStatus      `json:"storage"`
	ConfigAudits []InvoiceConfigAuditEntry `json:"config_audits"`
}

type InvoiceConfigAuditEntry struct {
	ID             string    `json:"id"`
	AdminID        int64     `json:"admin_id"`
	ChangedFields  []string  `json:"changed_fields"`
	PreviousEnable bool      `json:"previous_enabled"`
	Enabled        bool      `json:"enabled"`
	IPAddress      string    `json:"ip_address"`
	UserAgentHash  string    `json:"user_agent_hash"`
	CreatedAt      time.Time `json:"created_at"`
}

type InvoiceReconciliationEntry struct {
	FileID           int64  `json:"file_id"`
	InvoiceRequestID int64  `json:"invoice_request_id"`
	StorageKey       string `json:"storage_key"`
	SHA256           string `json:"sha256"`
}

type InvoiceReconciliationReport struct {
	CheckedAt         time.Time                    `json:"checked_at"`
	DatabaseFileCount int                          `json:"database_file_count"`
	StorageFileCount  int                          `json:"storage_file_count"`
	MissingFiles      []InvoiceReconciliationEntry `json:"missing_files"`
	OrphanStorageKeys []string                     `json:"orphan_storage_keys"`
}

type InvoiceUploadInput struct {
	Filename string
	Reader   io.Reader
}

type InvoicePreparedFile struct {
	Metadata InvoiceFileMetadata
}

type InvoiceDownload struct {
	Reader   io.ReadCloser
	Metadata InvoiceFileMetadata
}

type InvoiceEligibleOrdersResult struct {
	Items             []InvoiceOrderSnapshot `json:"items"`
	Total             int64                  `json:"total"`
	Page              int                    `json:"page"`
	PageSize          int                    `json:"page_size"`
	IneligibleReasons map[string]int64       `json:"ineligible_reasons"`
}

type InvoiceCreateParams struct {
	UserID   int64
	OrderIDs []int64
	Header   InvoiceHeaderInput
	Config   InvoiceConfig
	Actor    InvoiceAuditActor
}

type InvoiceResubmitParams struct {
	RequestID int64
	UserID    int64
	Header    InvoiceHeaderInput
	Config    InvoiceConfig
	Actor     InvoiceAuditActor
}

type InvoiceAdminActionParams struct {
	RequestID int64
	AdminID   int64
	Reason    string
	Note      string
	Actor     InvoiceAuditActor
}

type InvoiceIssueParams struct {
	RequestID     int64
	AdminID       int64
	InvoiceCode   string
	InvoiceNumber string
	InvoiceDate   time.Time
	File          InvoiceFileMetadata
	Actor         InvoiceAuditActor
}

type InvoiceReplaceFileParams struct {
	RequestID   int64
	AdminID     int64
	Reason      string
	InvoiceDate time.Time
	File        InvoiceFileMetadata
	Actor       InvoiceAuditActor
}

// InvoiceRepository owns all invoice transactions. Payment/refund services are
// deliberately absent from this interface so the module cannot mutate them.
type InvoiceRepository interface {
	HasHistory(context.Context, int64) (bool, error)
	ListEligibleOrders(context.Context, int64, InvoiceConfig, int, int, string) (InvoiceEligibleOrdersResult, error)
	Create(context.Context, InvoiceCreateParams) (*InvoiceRequest, error)
	ListUser(context.Context, int64, int, int) ([]InvoiceRequest, int64, error)
	GetUser(context.Context, int64, int64) (*InvoiceRequest, error)
	Cancel(context.Context, int64, int64, InvoiceAuditActor) (*InvoiceRequest, error)
	Resubmit(context.Context, InvoiceResubmitParams) (*InvoiceRequest, error)
	ListAdmin(context.Context, InvoiceListParams) ([]InvoiceRequest, int64, error)
	GetAdmin(context.Context, int64) (*InvoiceRequest, error)
	Accept(context.Context, InvoiceAdminActionParams) (*InvoiceRequest, error)
	Release(context.Context, InvoiceAdminActionParams) (*InvoiceRequest, error)
	Reject(context.Context, InvoiceAdminActionParams) (*InvoiceRequest, error)
	Issue(context.Context, InvoiceIssueParams) (*InvoiceRequest, error)
	ReplaceFile(context.Context, InvoiceReplaceFileParams) (*InvoiceRequest, error)
	Void(context.Context, InvoiceAdminActionParams) (*InvoiceRequest, error)
	ListAuditLogs(context.Context, int64) ([]InvoiceAuditLog, error)
	GetCurrentFileForUser(context.Context, int64, int64) (*InvoiceRequest, *InvoiceFileMetadata, error)
	GetCurrentFileForAdmin(context.Context, int64) (*InvoiceRequest, *InvoiceFileMetadata, error)
	ListAllFiles(context.Context) ([]InvoiceFileMetadata, error)
}

type InvoiceEmailNotifier interface {
	ApplicationSubmitted(context.Context, *InvoiceRequest) error
	ApplicationRejected(context.Context, *InvoiceRequest) error
	InvoiceIssued(context.Context, *InvoiceRequest) error
}
