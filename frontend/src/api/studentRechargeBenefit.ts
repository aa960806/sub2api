import { apiClient } from './client'

/** Server-side configuration for the isolated student recharge offer. */
export interface StudentRechargeBenefitConfig {
  enabled: boolean
  bonus_rate: number
  min_recharge_amount: number
  per_order_cap: number
}

export interface StudentRechargeBenefitStatus extends StudentRechargeBenefitConfig {
  is_student: boolean
  can_use: boolean
}

export interface StudentRechargeBenefitQuote extends StudentRechargeBenefitStatus {
  recharge_amount: number
  base_amount: number
  bonus_amount: number
  total_amount: number
}

export interface StudentAccountAdminItem {
  user_id: number
  email: string
  username: string
  is_student: boolean
  granted_by?: number
  granted_at?: string
  revoked_by?: number
  revoked_at?: string
  revoke_reason: string
}

export interface StudentAccountAuditItem {
  id: number
  user_id: number
  user_email: string
  admin_user_id: number
  admin_email: string
  action: 'grant' | 'revoke'
  previous_is_student: boolean
  current_is_student: boolean
  reason: string
  client_ip: string
  created_at: string
}

export async function getStudentRechargeBenefitStatus(): Promise<StudentRechargeBenefitStatus> {
  const response = await apiClient.get<StudentRechargeBenefitStatus>('/activity/student-recharge/status')
  return response.data
}

export async function quoteStudentRechargeBenefit(rechargeAmount: number): Promise<StudentRechargeBenefitQuote> {
  const response = await apiClient.get<StudentRechargeBenefitQuote>('/activity/student-recharge/quote', {
    params: { amount: rechargeAmount },
  })
  return response.data
}

export async function getStudentRechargeBenefitConfig(): Promise<StudentRechargeBenefitConfig> {
  const response = await apiClient.get<StudentRechargeBenefitConfig>('/admin/activity/student-recharge/config')
  return response.data
}

export async function updateStudentRechargeBenefitConfig(
  payload: StudentRechargeBenefitConfig,
): Promise<StudentRechargeBenefitConfig> {
  const response = await apiClient.put<StudentRechargeBenefitConfig>('/admin/activity/student-recharge/config', payload)
  return response.data
}

export async function listStudentAccounts(keyword = '', limit = 50): Promise<StudentAccountAdminItem[]> {
  const response = await apiClient.get<StudentAccountAdminItem[]>('/admin/activity/student-recharge/users', {
    params: { keyword, limit },
  })
  return response.data
}

export async function grantStudentAccount(userId: number, reason = ''): Promise<StudentAccountAdminItem> {
  const response = await apiClient.post<StudentAccountAdminItem>(
    `/admin/activity/student-recharge/users/${userId}/grant`,
    { reason },
  )
  return response.data
}

export async function revokeStudentAccount(userId: number, reason = ''): Promise<StudentAccountAdminItem> {
  const response = await apiClient.post<StudentAccountAdminItem>(
    `/admin/activity/student-recharge/users/${userId}/revoke`,
    { reason },
  )
  return response.data
}

export async function listStudentAccountAuditLogs(limit = 100): Promise<StudentAccountAuditItem[]> {
  const response = await apiClient.get<StudentAccountAuditItem[]>('/admin/activity/student-recharge/audit-logs', {
    params: { limit },
  })
  return response.data
}
