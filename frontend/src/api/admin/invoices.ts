import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'
import type { InvoiceAdminConfigResult, InvoiceAuditLog, InvoiceConfig, InvoiceReconciliationReport, InvoiceRequest } from '@/types/invoice'

const keyHeaders = (key: string) => ({ headers: { 'Idempotency-Key': key } })

const adminInvoicesAPI = {
  getConfig: () => apiClient.get<InvoiceAdminConfigResult>('/admin/invoices/config').then(response => response.data),
  updateConfig: (config: InvoiceConfig) => apiClient.put<InvoiceAdminConfigResult>('/admin/invoices/config', config).then(response => response.data),
  list: (params?: Record<string, string | number | undefined>) =>
    apiClient.get<BasePaginationResponse<InvoiceRequest>>('/admin/invoices', { params }).then(response => response.data),
  get: (id: number) => apiClient.get<InvoiceRequest>(`/admin/invoices/${id}`).then(response => response.data),
  accept: (id: number, note: string, key: string) =>
    apiClient.post<InvoiceRequest>(`/admin/invoices/${id}/accept`, { note }, keyHeaders(key)).then(response => response.data),
  release: (id: number, reason: string, key: string) =>
    apiClient.post<InvoiceRequest>(`/admin/invoices/${id}/release`, { reason }, keyHeaders(key)).then(response => response.data),
  reject: (id: number, reason: string, key: string) =>
    apiClient.post<InvoiceRequest>(`/admin/invoices/${id}/reject`, { reason }, keyHeaders(key)).then(response => response.data),
  issue(id: number, input: { file: File; invoice_date: string; invoice_code: string; invoice_number: string }) {
    const body = new FormData()
    body.append('file', input.file)
    body.append('invoice_date', input.invoice_date)
    body.append('invoice_code', input.invoice_code)
    body.append('invoice_number', input.invoice_number)
    return apiClient.post<InvoiceRequest>(`/admin/invoices/${id}/issue`, body, { headers: { 'Content-Type': 'multipart/form-data' } }).then(response => response.data)
  },
  replaceFile(id: number, input: { file: File; invoice_date: string; reason: string }) {
    const body = new FormData()
    body.append('file', input.file)
    body.append('invoice_date', input.invoice_date)
    body.append('reason', input.reason)
    return apiClient.post<InvoiceRequest>(`/admin/invoices/${id}/replace-file`, body, { headers: { 'Content-Type': 'multipart/form-data' } }).then(response => response.data)
  },
  voidInvoice: (id: number, reason: string, key: string) =>
    apiClient.post<InvoiceRequest>(`/admin/invoices/${id}/void`, { reason }, keyHeaders(key)).then(response => response.data),
  resendEmail: (id: number, key: string) =>
    apiClient.post<InvoiceRequest>(`/admin/invoices/${id}/resend-email`, {}, keyHeaders(key)).then(response => response.data),
  download: (id: number) => apiClient.get<Blob>(`/admin/invoices/${id}/download`, { responseType: 'blob' }),
  listAuditLogs: (id: number) => apiClient.get<InvoiceAuditLog[]>(`/admin/invoices/${id}/audit-logs`).then(response => response.data),
  reconcileFiles: () => apiClient.get<InvoiceReconciliationReport>('/admin/invoices/reconciliation').then(response => response.data),
}

export default adminInvoicesAPI
