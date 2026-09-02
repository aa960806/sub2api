import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'
import type { InvoiceEligibleOrdersResult, InvoiceHeaderInput, InvoicePublicConfig, InvoiceRequest } from '@/types/invoice'

export function createInvoiceIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  return `invoice-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

export function shouldShowInvoiceMenu(config: Pick<InvoicePublicConfig, 'enabled' | 'has_history'>): boolean {
  return config.enabled || config.has_history
}

const idempotencyHeaders = (key: string) => ({ headers: { 'Idempotency-Key': key } })

export const invoicesAPI = {
  getConfig: () => apiClient.get<InvoicePublicConfig>('/invoices/config').then(response => response.data),
  getEligibleOrders: (params?: { page?: number; page_size?: number; keyword?: string }) =>
    apiClient.get<InvoiceEligibleOrdersResult>('/invoices/eligible-orders', { params }).then(response => response.data),
  listMy: (params?: { page?: number; page_size?: number }) =>
    apiClient.get<BasePaginationResponse<InvoiceRequest>>('/invoices/my', { params }).then(response => response.data),
  getMy: (id: number) => apiClient.get<InvoiceRequest>(`/invoices/${id}`).then(response => response.data),
  create: (orderIds: number[], header: InvoiceHeaderInput, key: string) =>
    apiClient.post<InvoiceRequest>('/invoices', { order_ids: orderIds, ...header }, idempotencyHeaders(key)).then(response => response.data),
  cancel: (id: number, key: string) =>
    apiClient.post<InvoiceRequest>(`/invoices/${id}/cancel`, {}, idempotencyHeaders(key)).then(response => response.data),
  resubmit: (id: number, header: InvoiceHeaderInput, key: string) =>
    apiClient.put<InvoiceRequest>(`/invoices/${id}/resubmit`, header, idempotencyHeaders(key)).then(response => response.data),
  download: (id: number) => apiClient.get<Blob>(`/invoices/${id}/download`, { responseType: 'blob' }),
}
