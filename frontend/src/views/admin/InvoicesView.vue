<template>
  <AppLayout>
    <div class="space-y-5">
      <header class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div><h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('invoice.admin.title') }}</h1><p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('invoice.admin.description') }}</p></div>
        <div class="inline-flex w-fit rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900">
          <button class="px-4 py-2 text-sm font-medium" :class="tab === 'requests' ? activeTabClass : inactiveTabClass" @click="tab = 'requests'">{{ t('invoice.admin.requestsTab') }}</button>
          <button class="px-4 py-2 text-sm font-medium" :class="tab === 'settings' ? activeTabClass : inactiveTabClass" @click="tab = 'settings'">{{ t('invoice.admin.settingsTab') }}</button>
        </div>
      </header>

      <template v-if="tab === 'requests'">
        <section class="card p-4">
          <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-[160px_1fr_1fr_1fr_auto]">
            <select v-model="filters.status" class="input" @change="search"><option value="">{{ t('common.all') }}</option><option v-for="status in statuses" :key="status" :value="status">{{ statusLabel(status) }}</option></select>
            <input v-model="filters.request_no" class="input" :placeholder="t('invoice.admin.searchRequestNo')" @keyup.enter="search" />
            <input v-model="filters.user_email" class="input" :placeholder="t('invoice.admin.searchEmail')" @keyup.enter="search" />
            <input v-model="filters.order_no" class="input" :placeholder="t('invoice.admin.searchOrderNo')" @keyup.enter="search" />
            <div class="flex gap-2"><button class="btn btn-primary" @click="search"><Icon name="search" size="sm" />{{ t('common.search') }}</button><button class="btn btn-secondary" :title="t('common.refresh')" :disabled="loading" @click="loadRequests"><Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" /></button></div>
          </div>
        </section>

        <section class="card overflow-hidden">
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/80"><tr><th class="table-th">{{ t('invoice.fields.requestNo') }}</th><th class="table-th">{{ t('invoice.admin.applicant') }}</th><th class="table-th">{{ t('invoice.fields.titleName') }}</th><th class="table-th">{{ t('invoice.fields.status') }}</th><th class="table-th text-right">{{ t('invoice.fields.amount') }}</th><th class="table-th">{{ t('invoice.fields.createdAt') }}</th><th class="table-th text-right">{{ t('common.actions') }}</th></tr></thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr v-if="loading"><td colspan="7" class="px-5 py-14 text-center text-sm text-gray-500">{{ t('common.loading') }}</td></tr>
                <tr v-else-if="requests.length === 0"><td colspan="7" class="px-5 py-14 text-center text-sm text-gray-500">{{ t('invoice.admin.noRequests') }}</td></tr>
                <tr v-for="request in requests" :key="request.id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60">
                  <td class="table-td font-mono text-xs">{{ request.request_no }}</td>
                  <td class="table-td"><div class="font-medium text-gray-900 dark:text-white">{{ request.user_name || '-' }}</div><div class="text-xs text-gray-500">{{ request.user_email }}</div></td>
                  <td class="table-td"><div>{{ request.title_name }}</div><div v-if="request.taxpayer_id" class="font-mono text-xs text-gray-500">{{ request.taxpayer_id }}</div></td>
                  <td class="table-td"><span class="badge" :class="statusClass(request.status)">{{ statusLabel(request.status) }}</span></td>
                  <td class="table-td text-right font-medium">¥{{ request.total_amount }}</td><td class="table-td">{{ formatDateTime(request.created_at) }}</td>
                  <td class="table-td text-right"><button class="btn-icon" :title="t('common.view')" @click="openDetail(request.id)"><Icon name="eye" size="sm" /></button></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="total > pageSize" class="border-t border-gray-200 px-5 py-3 dark:border-dark-700"><Pagination :page="page" :total="total" :page-size="pageSize" @update:page="changePage" @update:pageSize="changePageSize" /></div>
        </section>
      </template>

      <template v-else>
        <section class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
          <form class="card space-y-5 p-5" @submit.prevent="saveConfig">
            <div class="flex items-center justify-between"><div><h2 class="font-semibold text-gray-900 dark:text-white">{{ t('invoice.admin.businessSettings') }}</h2><p class="mt-1 text-xs text-gray-500">{{ t('invoice.admin.defaultOff') }}</p></div><label class="inline-flex items-center gap-2 text-sm font-medium"><input v-model="config.enabled" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />{{ t('invoice.admin.enabled') }}</label></div>
            <div class="grid gap-4 md:grid-cols-2">
              <label class="block"><span class="input-label">{{ t('invoice.admin.minAmount') }}</span><input v-model.number="config.min_amount" type="number" min="0.01" step="0.01" class="input" /></label>
              <label class="block"><span class="input-label">{{ t('invoice.admin.maxAmount') }}</span><input v-model.number="config.max_amount" type="number" min="0" step="0.01" class="input" /></label>
              <label class="block"><span class="input-label">{{ t('invoice.admin.applicationDays') }}</span><input v-model.number="config.application_days" type="number" min="0" max="3650" step="1" class="input" /></label>
              <label class="block"><span class="input-label">{{ t('invoice.admin.maxOrders') }}</span><input v-model.number="config.max_orders_per_request" type="number" min="1" max="100" step="1" class="input" /></label>
              <label class="block"><span class="input-label">{{ t('invoice.fields.itemName') }}</span><input v-model="config.item_name" class="input" maxlength="100" /></label>
              <label class="block"><span class="input-label">{{ t('invoice.admin.maxFileSize') }}</span><input v-model.number="config.max_file_size_mb" type="number" min="1" max="20" step="1" class="input" /></label>
            </div>
            <label class="block"><span class="input-label">{{ t('invoice.admin.notificationEmails') }}</span><textarea v-model="notificationEmailsText" class="input min-h-24 resize-y" :placeholder="t('invoice.admin.notificationEmailsHint')" /></label>
            <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200"><input v-model="config.allow_reapply_after_void" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />{{ t('invoice.admin.allowReapplyAfterVoid') }}</label>
            <div class="flex justify-end"><button class="btn btn-primary" :disabled="savingConfig || !configValid">{{ savingConfig ? t('common.processing') : t('common.save') }}</button></div>
          </form>

          <aside class="space-y-5">
            <section class="card p-5"><h2 class="font-semibold text-gray-900 dark:text-white">{{ t('invoice.admin.storage') }}</h2><div class="mt-4 flex items-center gap-2"><span class="h-2.5 w-2.5 rounded-full" :class="storage.available ? 'bg-emerald-500' : 'bg-red-500'"></span><span class="text-sm font-medium">{{ storage.available ? t('invoice.admin.storageReady') : t('invoice.admin.storageUnavailable') }}</span></div><p class="mt-2 text-sm text-gray-500">{{ t('invoice.admin.freeSpace') }}: {{ formatBytes(storage.free_bytes) }}</p><p v-if="storage.failure_reason" class="mt-2 text-xs text-red-600">{{ storage.failure_reason }}</p><button class="btn btn-secondary mt-4 w-full" :disabled="reconciling" @click="runReconciliation"><Icon name="database" size="sm" />{{ t('invoice.admin.reconcile') }}</button></section>
            <section v-if="reconciliation" class="card p-5"><h2 class="font-semibold text-gray-900 dark:text-white">{{ t('invoice.admin.reconcileResult') }}</h2><dl class="mt-3 space-y-2 text-sm"><div class="flex justify-between"><dt>{{ t('invoice.admin.dbFiles') }}</dt><dd>{{ reconciliation.database_file_count }}</dd></div><div class="flex justify-between"><dt>{{ t('invoice.admin.diskFiles') }}</dt><dd>{{ reconciliation.storage_file_count }}</dd></div><div class="flex justify-between"><dt>{{ t('invoice.admin.missingFiles') }}</dt><dd :class="reconciliation.missing_files.length ? 'text-red-600' : 'text-emerald-600'">{{ reconciliation.missing_files.length }}</dd></div><div class="flex justify-between"><dt>{{ t('invoice.admin.orphanFiles') }}</dt><dd :class="reconciliation.orphan_storage_keys.length ? 'text-amber-600' : 'text-emerald-600'">{{ reconciliation.orphan_storage_keys.length }}</dd></div></dl></section>
          </aside>
        </section>

        <section class="card overflow-hidden">
          <div class="border-b border-gray-200 px-5 py-4 dark:border-dark-700"><h2 class="font-semibold text-gray-900 dark:text-white">{{ t('invoice.admin.configAudit') }}</h2></div>
          <div class="overflow-x-auto"><table class="min-w-full"><thead><tr><th class="table-th">{{ t('invoice.fields.createdAt') }}</th><th class="table-th">{{ t('invoice.admin.adminId') }}</th><th class="table-th">{{ t('invoice.admin.changedFields') }}</th><th class="table-th">{{ t('invoice.admin.enabled') }}</th></tr></thead><tbody><tr v-if="configAudits.length === 0"><td colspan="4" class="px-5 py-8 text-center text-sm text-gray-500">{{ t('invoice.admin.noAudit') }}</td></tr><tr v-for="audit in configAudits" :key="audit.id"><td class="table-td">{{ formatDateTime(audit.created_at) }}</td><td class="table-td">#{{ audit.admin_id }}</td><td class="table-td">{{ audit.changed_fields.join(', ') || '-' }}</td><td class="table-td"><span :class="audit.enabled ? 'text-emerald-600' : 'text-gray-500'">{{ audit.enabled ? t('common.enabled') : t('common.disabled') }}</span></td></tr></tbody></table></div>
        </section>
      </template>
    </div>

    <BaseDialog :show="!!detail" :title="detail?.request_no || t('invoice.admin.detail')" width="extra-wide" @close="closeDetail">
      <div v-if="detail" class="space-y-5">
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-5"><div><p class="text-xs text-gray-500">{{ t('invoice.fields.status') }}</p><span class="badge mt-1" :class="statusClass(detail.status)">{{ statusLabel(detail.status) }}</span></div><div><p class="text-xs text-gray-500">{{ t('invoice.fields.amount') }}</p><p class="mt-1 font-semibold">¥{{ detail.total_amount }}</p></div><div><p class="text-xs text-gray-500">{{ t('invoice.fields.orderCount') }}</p><p class="mt-1 font-semibold">{{ detail.order_count }}</p></div><div><p class="text-xs text-gray-500">{{ t('invoice.fields.invoiceDate') }}</p><p class="mt-1 font-semibold">{{ formatDate(detail.invoice_date) }}</p></div><div><p class="text-xs text-gray-500">{{ t('invoice.fields.createdAt') }}</p><p class="mt-1 text-sm">{{ formatDateTime(detail.created_at) }}</p></div></div>
        <div v-if="detail.reject_reason" class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">{{ detail.reject_reason }}</div>
        <div class="grid gap-5 lg:grid-cols-2">
          <section><h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('invoice.admin.invoiceInfo') }}</h3><dl class="mt-3 grid gap-3 text-sm sm:grid-cols-2"><div><dt class="text-gray-500">{{ t('invoice.fields.titleName') }}</dt><dd class="mt-1">{{ detail.title_name }}</dd></div><div><dt class="text-gray-500">{{ t('invoice.fields.taxpayerId') }}</dt><dd class="mt-1 font-mono">{{ detail.taxpayer_id || '-' }}</dd></div><div><dt class="text-gray-500">{{ t('invoice.fields.recipientEmail') }}</dt><dd class="mt-1">{{ detail.recipient_email }}</dd></div><div><dt class="text-gray-500">{{ t('invoice.fields.recipientPhone') }}</dt><dd class="mt-1">{{ detail.recipient_phone || '-' }}</dd></div><div><dt class="text-gray-500">{{ t('invoice.fields.bankName') }}</dt><dd class="mt-1">{{ detail.bank_name || '-' }}</dd></div><div><dt class="text-gray-500">{{ t('invoice.fields.bankAccount') }}</dt><dd class="mt-1 font-mono">{{ detail.bank_account || '-' }}</dd></div></dl></section>
          <section><h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('invoice.admin.issueInfo') }}</h3><dl class="mt-3 grid gap-3 text-sm sm:grid-cols-2"><div><dt class="text-gray-500">{{ t('invoice.fields.invoiceCode') }}</dt><dd class="mt-1 font-mono">{{ detail.invoice_code || '-' }}</dd></div><div><dt class="text-gray-500">{{ t('invoice.fields.invoiceNumber') }}</dt><dd class="mt-1 font-mono">{{ detail.invoice_number || '-' }}</dd></div><div><dt class="text-gray-500">{{ t('invoice.fields.userNote') }}</dt><dd class="mt-1">{{ detail.user_note || '-' }}</dd></div><div><dt class="text-gray-500">{{ t('invoice.fields.adminNote') }}</dt><dd class="mt-1">{{ detail.admin_note || '-' }}</dd></div></dl></section>
        </div>
        <div class="overflow-x-auto"><table class="min-w-full"><thead><tr><th class="table-th">{{ t('invoice.fields.orderNo') }}</th><th class="table-th">{{ t('invoice.fields.orderType') }}</th><th class="table-th">{{ t('invoice.fields.paidAt') }}</th><th class="table-th text-right">{{ t('invoice.fields.amount') }}</th></tr></thead><tbody><tr v-for="order in detail.orders || []" :key="order.id"><td class="table-td font-mono text-xs">{{ order.out_trade_no }}</td><td class="table-td">{{ orderTypeLabel(order.order_type) }}</td><td class="table-td">{{ formatDateTime(order.completed_at || order.paid_at) }}</td><td class="table-td text-right">¥{{ order.pay_amount }}</td></tr></tbody></table></div>
        <section><h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('invoice.admin.auditLog') }}</h3><div class="mt-3 max-h-44 overflow-y-auto border-l border-gray-200 pl-4 dark:border-dark-700"><div v-if="auditLogs.length === 0" class="py-3 text-sm text-gray-500">{{ t('invoice.admin.noAudit') }}</div><div v-for="log in auditLogs" :key="log.id" class="mb-3 text-sm"><p class="font-medium">{{ t(`invoice.audit.${log.action}`, log.action) }}</p><p class="text-xs text-gray-500">{{ formatDateTime(log.created_at) }} · {{ log.actor_type }}{{ log.actor_id ? ` #${log.actor_id}` : '' }}</p></div></div></section>
      </div>
      <template #footer><div class="flex flex-wrap justify-end gap-2"><button v-if="detail?.current_file" class="btn btn-secondary" @click="downloadInvoice"><Icon name="download" size="sm" />{{ t('invoice.actions.download') }}</button><button v-if="detail?.status === 'PENDING'" class="btn btn-primary" @click="openConfirmAction('accept')">{{ t('invoice.actions.accept') }}</button><button v-if="detail?.status === 'PROCESSING'" class="btn btn-secondary" @click="openReasonAction('release')">{{ t('invoice.actions.release') }}</button><button v-if="detail?.status === 'PENDING' || detail?.status === 'PROCESSING'" class="btn btn-secondary" @click="openReasonAction('reject')">{{ t('invoice.actions.reject') }}</button><button v-if="detail?.status === 'PROCESSING'" class="btn btn-primary" @click="openUpload('issue')"><Icon name="upload" size="sm" />{{ t('invoice.actions.issue') }}</button><button v-if="detail?.status === 'ISSUED'" class="btn btn-secondary" @click="openUpload('replace')">{{ t('invoice.actions.replaceFile') }}</button><button v-if="detail?.status === 'ISSUED'" class="btn btn-danger" @click="openReasonAction('void')">{{ t('invoice.actions.void') }}</button><button class="btn btn-secondary" @click="runImmediateAction('resend')"><Icon name="mail" size="sm" />{{ t('invoice.actions.resendEmail') }}</button></div></template>
    </BaseDialog>

    <BaseDialog :show="confirmAction === 'accept'" :title="t('invoice.actions.accept')" width="narrow" @close="confirmAction = null">
      <p class="text-sm text-gray-700 dark:text-dark-200">{{ t('invoice.admin.confirmAccept') }}</p>
      <template #footer><button class="btn btn-primary" :disabled="actionLoading" @click="runConfirmedAction">{{ t('common.confirm') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="!!reasonAction" :title="reasonAction ? t(`invoice.actions.${reasonAction}`) : ''" width="narrow" @close="reasonAction = null">
      <div class="space-y-4"><div v-if="reasonAction === 'void'" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200">{{ voidWarning }}</div><label class="block"><span class="input-label">{{ t('invoice.fields.reason') }}</span><textarea v-model="reasonText" class="input min-h-24 resize-y" maxlength="1000" /></label></div>
      <template #footer><button class="btn" :class="reasonAction === 'void' ? 'btn-danger' : 'btn-primary'" :disabled="actionLoading || !reasonText.trim()" @click="runReasonAction">{{ t('common.confirm') }}</button></template>
    </BaseDialog>

    <BaseDialog :show="!!uploadMode" :title="uploadMode === 'issue' ? t('invoice.actions.issue') : t('invoice.actions.replaceFile')" width="normal" @close="uploadMode = null">
      <div class="space-y-4"><label class="block"><span class="input-label">{{ t('invoice.fields.invoiceDate') }}</span><input v-model="uploadForm.invoice_date" class="input" type="date" :max="today" /></label><template v-if="uploadMode === 'issue'"><label class="block"><span class="input-label">{{ t('invoice.fields.invoiceCode') }}</span><input v-model="uploadForm.invoice_code" class="input" maxlength="64" /></label><label class="block"><span class="input-label">{{ t('invoice.fields.invoiceNumber') }}</span><input v-model="uploadForm.invoice_number" class="input" maxlength="128" /></label></template><label v-else class="block"><span class="input-label">{{ t('invoice.fields.reason') }}</span><textarea v-model="uploadForm.reason" class="input min-h-20 resize-y" maxlength="1000" /></label><label class="block"><span class="input-label">{{ t('invoice.fields.file') }}</span><input class="input" type="file" accept=".pdf,.ofd,application/pdf,application/ofd" @change="selectFile" /></label><div v-if="selectedFile" class="rounded-lg border border-gray-200 px-3 py-2 text-xs dark:border-dark-700"><p class="font-medium text-gray-800 dark:text-dark-100">{{ selectedFile.name }} · {{ formatBytes(selectedFile.size) }}</p><p v-if="fileHashing" class="mt-1 text-gray-500">{{ t('invoice.admin.hashingFile') }}</p><p v-else-if="selectedFileHash" class="mt-1 break-all font-mono text-gray-500">{{ t('invoice.admin.fileSha256') }}: {{ selectedFileHash }}</p><p v-else-if="fileHashError" class="mt-1 text-amber-700 dark:text-amber-300">{{ t('invoice.admin.hashUnavailable') }}</p></div><p class="text-xs text-gray-500">{{ t('invoice.admin.fileHint', { size: detail?.config_snapshot?.max_file_size_mb || config.max_file_size_mb }) }}</p></div>
      <template #footer><button class="btn btn-primary" :disabled="uploading || !uploadValid" @click="submitUpload">{{ uploading ? t('common.processing') : t('common.confirm') }}</button></template>
    </BaseDialog>
    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpBlocked, isStepUpCancelled, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import adminInvoicesAPI from '@/api/admin/invoices'
import { createInvoiceIdempotencyKey } from '@/api/invoices'
import type { InvoiceAuditLog, InvoiceConfig, InvoiceConfigAuditEntry, InvoiceReconciliationReport, InvoiceRequest, InvoiceStatus, InvoiceStorageStatus } from '@/types/invoice'

const { t } = useI18n(); const appStore = useAppStore(); const stepUp = useStepUp()
const statuses: InvoiceStatus[] = ['PENDING', 'PROCESSING', 'REJECTED', 'ISSUED', 'CANCELLED', 'VOIDED']
const tab = ref<'requests' | 'settings'>('requests'), loading = ref(false), savingConfig = ref(false), reconciling = ref(false), actionLoading = ref(false), uploading = ref(false)
const requests = ref<InvoiceRequest[]>([]), total = ref(0), page = ref(1), pageSize = ref(20), detail = ref<InvoiceRequest | null>(null), auditLogs = ref<InvoiceAuditLog[]>([])
const filters = reactive({ status: '', request_no: '', user_email: '', order_no: '' })
const config = reactive<InvoiceConfig>({ enabled: false, min_amount: 0.01, max_amount: 0, application_days: 0, max_orders_per_request: 50, item_name: '', admin_notification_emails: [], max_file_size_mb: 10, allow_reapply_after_void: false })
const storage = reactive<InvoiceStorageStatus>({ available: false, free_bytes: 0, checked_at: '' }), configAudits = ref<InvoiceConfigAuditEntry[]>([]), notificationEmailsText = ref(''), reconciliation = ref<InvoiceReconciliationReport | null>(null)
const reasonAction = ref<'release' | 'reject' | 'void' | null>(null), confirmAction = ref<'accept' | null>(null), reasonText = ref(''), uploadMode = ref<'issue' | 'replace' | null>(null), selectedFile = ref<File | null>(null)
const selectedFileHash = ref(''), fileHashing = ref(false), fileHashError = ref(false)
const uploadForm = reactive({ invoice_date: '', invoice_code: '', invoice_number: '', reason: '' })
let actionKey = ''
const activeTabClass = 'rounded-md bg-primary-600 text-white shadow-sm', inactiveTabClass = 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
const today = new Date().toISOString().slice(0, 10)
const configValid = computed(() => config.min_amount > 0 && config.max_amount >= 0 && (config.max_amount === 0 || config.max_amount >= config.min_amount) && config.application_days >= 0 && config.max_orders_per_request >= 1 && config.max_orders_per_request <= 100 && !!config.item_name.trim() && config.max_file_size_mb >= 1 && config.max_file_size_mb <= 20)
const uploadValid = computed(() => !!selectedFile.value && !fileHashing.value && !!uploadForm.invoice_date && (uploadMode.value === 'replace' ? !!uploadForm.reason.trim() : !!uploadForm.invoice_number.trim()))
const voidWarning = computed(() => detail.value?.config_snapshot?.allow_reapply_after_void === true ? t('invoice.admin.voidReleases') : t('invoice.admin.voidKeepsReservation'))

async function loadRequests() { loading.value = true; try { const data = await adminInvoicesAPI.list({ page: page.value, page_size: pageSize.value, status: filters.status || undefined, request_no: filters.request_no.trim() || undefined, user_email: filters.user_email.trim() || undefined, order_no: filters.order_no.trim() || undefined }); requests.value = data.items || []; total.value = data.total || 0 } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { loading.value = false } }
async function loadConfig() { try { const data = await adminInvoicesAPI.getConfig(); Object.assign(config, data.config); Object.assign(storage, data.storage); configAudits.value = data.config_audits || []; notificationEmailsText.value = config.admin_notification_emails.join('\n') } catch (error: any) { appStore.showError(error?.message || t('common.error')) } }
function reportStepUpError(error: unknown): boolean {
  if (isStepUpCancelled(error)) return true
  if (isStepUpBlocked(error)) {
    appStore.showError(stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled'))
    return true
  }
  return false
}
async function saveConfig() { if (!configValid.value) return; savingConfig.value = true; try { config.admin_notification_emails = notificationEmailsText.value.split(/[\n,;]/).map(value => value.trim()).filter(Boolean); const data = await stepUp.run(() => adminInvoicesAPI.updateConfig({ ...config })); Object.assign(config, data.config); Object.assign(storage, data.storage); configAudits.value = data.config_audits || []; notificationEmailsText.value = config.admin_notification_emails.join('\n'); window.dispatchEvent(new CustomEvent('invoice-config-changed')); appStore.showSuccess(t('invoice.admin.configSaved')) } catch (error: any) { if (!reportStepUpError(error)) appStore.showError(error?.message || t('common.error')) } finally { savingConfig.value = false } }
async function runReconciliation() { reconciling.value = true; try { reconciliation.value = await adminInvoicesAPI.reconcileFiles(); appStore.showSuccess(t('invoice.admin.reconcileDone')) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } finally { reconciling.value = false } }
function search() { page.value = 1; void loadRequests() }
function changePage(value: number) { page.value = value; void loadRequests() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void loadRequests() }
async function openDetail(id: number) { try { const [request, logs] = await Promise.all([adminInvoicesAPI.get(id), adminInvoicesAPI.listAuditLogs(id)]); detail.value = request; auditLogs.value = logs || [] } catch (error: any) { appStore.showError(error?.message || t('common.error')) } }
function closeDetail() { detail.value = null; auditLogs.value = [] }
async function refreshDetail() { if (!detail.value) return; await openDetail(detail.value.id); await loadRequests() }
function openReasonAction(action: 'release' | 'reject' | 'void') { reasonAction.value = action; reasonText.value = ''; actionKey = '' }
function openConfirmAction(action: 'accept') { confirmAction.value = action; actionKey = '' }
async function runImmediateAction(action: 'accept' | 'resend'): Promise<boolean> { if (!detail.value) return false; actionLoading.value = true; actionKey ||= createInvoiceIdempotencyKey(); try { await stepUp.run(() => action === 'accept' ? adminInvoicesAPI.accept(detail.value!.id, '', actionKey) : adminInvoicesAPI.resendEmail(detail.value!.id, actionKey)); actionKey = ''; appStore.showSuccess(t('common.success')); await refreshDetail(); return true } catch (error: any) { if (!reportStepUpError(error)) appStore.showError(error?.message || t('common.error')); return false } finally { actionLoading.value = false } }
async function runConfirmedAction() { if (confirmAction.value === 'accept' && await runImmediateAction('accept')) confirmAction.value = null }
async function runReasonAction() { if (!detail.value || !reasonAction.value || !reasonText.value.trim()) return; actionLoading.value = true; actionKey ||= createInvoiceIdempotencyKey(); try { const id = detail.value.id, reason = reasonText.value.trim(), action = reasonAction.value; await stepUp.run(() => action === 'release' ? adminInvoicesAPI.release(id, reason, actionKey) : action === 'reject' ? adminInvoicesAPI.reject(id, reason, actionKey) : adminInvoicesAPI.voidInvoice(id, reason, actionKey)); actionKey = ''; reasonAction.value = null; appStore.showSuccess(t('common.success')); await refreshDetail() } catch (error: any) { if (!reportStepUpError(error)) appStore.showError(error?.message || t('common.error')) } finally { actionLoading.value = false } }
function openUpload(mode: 'issue' | 'replace') { uploadMode.value = mode; selectedFile.value = null; selectedFileHash.value = ''; fileHashError.value = false; fileHashing.value = false; Object.assign(uploadForm, { invoice_date: detail.value?.invoice_date?.slice(0, 10) || today, invoice_code: detail.value?.invoice_code || '', invoice_number: detail.value?.invoice_number || '', reason: '' }) }
async function selectFile(event: Event) { const file = (event.target as HTMLInputElement).files?.[0] || null; selectedFile.value = file; selectedFileHash.value = ''; fileHashError.value = false; if (!file) return; fileHashing.value = true; try { if (!globalThis.crypto?.subtle) throw new Error('SHA-256 unavailable'); const digest = await globalThis.crypto.subtle.digest('SHA-256', await file.arrayBuffer()); if (selectedFile.value === file) selectedFileHash.value = Array.from(new Uint8Array(digest), value => value.toString(16).padStart(2, '0')).join('') } catch { if (selectedFile.value === file) fileHashError.value = true } finally { if (selectedFile.value === file) fileHashing.value = false } }
async function submitUpload() { if (!detail.value || !selectedFile.value || !uploadValid.value || !uploadMode.value) return; uploading.value = true; try { const id = detail.value.id, file = selectedFile.value, mode = uploadMode.value; await stepUp.run(() => mode === 'issue' ? adminInvoicesAPI.issue(id, { file, invoice_date: uploadForm.invoice_date, invoice_code: uploadForm.invoice_code, invoice_number: uploadForm.invoice_number }) : adminInvoicesAPI.replaceFile(id, { file, invoice_date: uploadForm.invoice_date, reason: uploadForm.reason.trim() })); uploadMode.value = null; appStore.showSuccess(t('common.success')); await refreshDetail() } catch (error: any) { if (!reportStepUpError(error)) appStore.showError(error?.message || t('common.error')) } finally { uploading.value = false } }
async function downloadInvoice() { if (!detail.value) return; try { const response = await adminInvoicesAPI.download(detail.value.id); saveBlob(response.data, detail.value.current_file?.original_filename || `${detail.value.request_no}.pdf`) } catch (error: any) { appStore.showError(error?.message || t('common.error')) } }
function saveBlob(blob: Blob, filename: string) { const url = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = url; link.download = filename; link.click(); URL.revokeObjectURL(url) }
function formatDateTime(value?: string) { return value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-' }
function formatDate(value?: string) { return value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(value)) : '-' }
function formatBytes(value: number) { if (!Number.isFinite(value) || value <= 0) return '0 B'; const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']; const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1); return `${(value / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}` }
function statusLabel(value: InvoiceStatus) { return t(`invoice.status.${value}`) }
function statusClass(value: InvoiceStatus) { return ({ PENDING: 'badge-warning', PROCESSING: 'badge-info', REJECTED: 'badge-danger', CANCELLED: 'badge-gray', ISSUED: 'badge-success', VOIDED: 'badge-gray' } as Record<InvoiceStatus, string>)[value] }
function orderTypeLabel(value: string) { return t(`invoice.orderType.${value}`, value) }
onMounted(async () => { await Promise.all([loadRequests(), loadConfig()]) })
</script>
