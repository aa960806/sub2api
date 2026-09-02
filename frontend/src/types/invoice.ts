export type InvoiceStatus = 'PENDING' | 'PROCESSING' | 'REJECTED' | 'CANCELLED' | 'ISSUED' | 'VOIDED'
export type InvoiceTitleType = 'PERSONAL' | 'COMPANY'

export interface InvoiceConfig {
  enabled: boolean
  min_amount: number
  max_amount: number
  application_days: number
  max_orders_per_request: number
  item_name: string
  admin_notification_emails: string[]
  max_file_size_mb: number
  allow_reapply_after_void: boolean
}

export interface InvoicePublicConfig extends InvoiceConfig {
  has_history: boolean
  allowed_order_types: string[]
}

export interface InvoiceHeaderInput {
  title_type: InvoiceTitleType
  title_name: string
  taxpayer_id: string
  recipient_email: string
  recipient_phone: string
  company_address: string
  company_phone: string
  bank_name: string
  bank_account: string
  user_note: string
}

export interface InvoiceOrderSnapshot {
  id: number
  invoice_request_id?: number
  payment_order_id: number
  out_trade_no: string
  order_type: string
  payment_type: string
  pay_amount: string
  currency: string
  paid_at?: string
  completed_at?: string
  application_anchor?: string
  application_deadline?: string
  reservation_active: boolean
  released_at?: string
}

export interface InvoiceFileMetadata {
  id: number
  invoice_request_id: number
  original_filename: string
  content_type: string
  file_extension: string
  file_size: number
  sha256: string
  is_current: boolean
  uploaded_by?: number
  uploaded_at: string
  replaced_at?: string
}

export interface InvoiceRequest {
  id: number
  request_no: string
  user_id: number
  user_email: string
  user_name: string
  status: InvoiceStatus
  title_type: InvoiceTitleType
  title_name: string
  taxpayer_id: string
  recipient_email: string
  recipient_phone: string
  company_address: string
  company_phone: string
  bank_name: string
  bank_account: string
  invoice_item_name: string
  currency: string
  total_amount: string
  order_count: number
  user_note: string
  admin_note: string
  reject_reason: string
  invoice_code: string
  invoice_number: string
  invoice_date?: string
  revision: number
  accepted_at?: string
  issued_at?: string
  rejected_at?: string
  cancelled_at?: string
  voided_at?: string
  created_at: string
  updated_at: string
  orders?: InvoiceOrderSnapshot[]
  current_file?: InvoiceFileMetadata
  config_snapshot?: InvoiceConfig
}

export interface InvoiceEligibleOrdersResult {
  items: InvoiceOrderSnapshot[]
  total: number
  page: number
  page_size: number
  ineligible_reasons: Record<string, number>
}

export interface InvoiceStorageStatus {
  available: boolean
  free_bytes: number
  checked_at: string
  failure_reason?: string
}

export interface InvoiceConfigAuditEntry {
  id: string
  admin_id: number
  changed_fields: string[]
  previous_enabled: boolean
  enabled: boolean
  ip_address: string
  user_agent_hash: string
  created_at: string
}

export interface InvoiceAdminConfigResult {
  config: InvoiceConfig
  storage: InvoiceStorageStatus
  config_audits: InvoiceConfigAuditEntry[]
}

export interface InvoiceAuditLog {
  id: number
  invoice_request_id: number
  request_no: string
  actor_type: 'user' | 'admin' | 'system'
  actor_id?: number
  action: string
  from_status: string
  to_status: string
  request_revision: number
  metadata: Record<string, unknown>
  ip_address: string
  created_at: string
}

export interface InvoiceReconciliationReport {
  checked_at: string
  database_file_count: number
  storage_file_count: number
  missing_files: Array<{ file_id: number; invoice_request_id: number; storage_key: string; sha256: string }>
  orphan_storage_keys: string[]
}
