<template>
  <section class="card overflow-hidden p-0" data-testid="student-recharge-admin">
    <header class="flex flex-col gap-4 border-b border-gray-200 px-5 py-5 dark:border-dark-700 sm:flex-row sm:items-start sm:justify-between">
      <div class="flex items-start gap-3">
        <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-emerald-50 text-emerald-600 dark:bg-emerald-950/30 dark:text-emerald-400">
          <Icon name="badge" size="md" aria-hidden="true" />
        </span>
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.studentRecharge.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.studentRecharge.description') }}</p>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-3">
        <div class="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-dark-200">
          <Toggle v-model="form.enabled" data-testid="student-benefit-enabled" />
          <span>{{ form.enabled ? t('common.enabled') : t('common.disabled') }}</span>
        </div>
        <button class="btn btn-primary inline-flex items-center gap-2" type="button" :disabled="loading || saving" data-testid="student-benefit-save" @click="saveConfig">
          <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="saving ? 'animate-spin' : ''" aria-hidden="true" />
          <span>{{ saving ? t('admin.studentRecharge.saving') : t('admin.studentRecharge.save') }}</span>
        </button>
      </div>
    </header>

    <div v-if="loading" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400" aria-busy="true">{{ t('admin.studentRecharge.loading') }}</div>
    <div v-else class="divide-y divide-gray-200 dark:divide-dark-700">
      <section class="space-y-4 px-5 py-5">
        <p v-if="!form.enabled" class="rounded-md border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">{{ t('admin.studentRecharge.disabledHint') }}</p>
        <p v-else class="rounded-md border border-emerald-200 bg-emerald-50/50 px-4 py-3 text-sm text-emerald-800 dark:border-emerald-800/60 dark:bg-emerald-950/20 dark:text-emerald-200">{{ t('admin.studentRecharge.switchHint') }}</p>
        <div class="grid gap-4 sm:grid-cols-3">
          <label class="space-y-1.5">
            <span class="input-label">{{ t('admin.studentRecharge.bonusRate') }}</span>
            <div class="relative">
              <input v-model.number="bonusPercent" class="input pr-9" type="number" min="0.01" max="1000" step="0.01" inputmode="decimal" data-testid="student-benefit-rate" />
              <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-gray-400">%</span>
            </div>
            <span class="input-hint">{{ t('admin.studentRecharge.bonusRateHint') }}</span>
          </label>
          <label class="space-y-1.5">
            <span class="input-label">{{ t('admin.studentRecharge.minimumAmount') }}</span>
            <input v-model.number="form.min_recharge_amount" class="input" type="number" min="0" step="0.01" inputmode="decimal" data-testid="student-benefit-minimum" />
          </label>
          <label class="space-y-1.5">
            <span class="input-label">{{ t('admin.studentRecharge.cap') }}</span>
            <input v-model.number="form.per_order_cap" class="input" type="number" min="0" step="0.01" inputmode="decimal" data-testid="student-benefit-cap" />
          </label>
        </div>
        <p class="rounded-md border border-gray-200 px-4 py-3 text-sm text-gray-600 dark:border-dark-700 dark:text-dark-300">
          {{ t('admin.studentRecharge.preview', { base: formatAmount(100), bonus: formatAmount(previewBonus) }) }}
        </p>
      </section>

      <section class="space-y-4 px-5 py-5">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.studentRecharge.accountsTitle') }}</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.studentRecharge.accountsHint') }}</p>
          </div>
          <div class="flex w-full gap-2 sm:w-auto">
            <input v-model.trim="keyword" class="input min-w-0 sm:w-72" type="search" :placeholder="t('admin.studentRecharge.searchPlaceholder')" data-testid="student-user-search" @keyup.enter="searchUsers" />
            <button class="btn-icon shrink-0" type="button" :title="t('admin.studentRecharge.search')" :aria-label="t('admin.studentRecharge.search')" :disabled="usersLoading" @click="searchUsers">
              <Icon name="search" size="sm" aria-hidden="true" />
            </button>
          </div>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full min-w-[860px]">
            <thead>
              <tr>
                <th class="table-th">{{ t('admin.studentRecharge.account') }}</th>
                <th class="table-th">{{ t('admin.studentRecharge.status') }}</th>
                <th class="table-th">{{ t('admin.studentRecharge.grantedAt') }}</th>
                <th class="table-th">{{ t('admin.studentRecharge.revokeReason') }}</th>
                <th class="table-th text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-if="usersLoading"><td colspan="5" class="px-4 py-8 text-center text-sm text-gray-400">{{ t('admin.studentRecharge.searching') }}</td></tr>
              <tr v-else-if="searched && !users.length"><td colspan="5" class="px-4 py-8 text-center text-sm text-gray-400">{{ t('admin.studentRecharge.noAccounts') }}</td></tr>
              <tr v-else-if="!searched"><td colspan="5" class="px-4 py-8 text-center text-sm text-gray-400">{{ t('admin.studentRecharge.searchPrompt') }}</td></tr>
              <tr v-for="item in users" :key="item.user_id">
                <td class="table-td">
                  <div class="font-medium text-gray-900 dark:text-white">{{ item.email }}</div>
                  <div class="mt-0.5 text-xs text-gray-400">#{{ item.user_id }} · {{ item.username || '-' }}</div>
                </td>
                <td class="table-td"><span :class="item.is_student ? 'badge badge-success' : 'badge badge-gray'">{{ item.is_student ? t('admin.studentRecharge.student') : t('admin.studentRecharge.regular') }}</span></td>
                <td class="table-td">{{ item.granted_at ? formatDate(item.granted_at) : '-' }}</td>
                <td class="table-td">
                  <input v-if="item.is_student" v-model.trim="revokeReasons[item.user_id]" class="input" maxlength="1000" :placeholder="t('admin.studentRecharge.revokeReasonPlaceholder')" />
                  <span v-else>{{ item.revoke_reason || '-' }}</span>
                </td>
                <td class="table-td text-right">
                  <button v-if="item.is_student" class="btn btn-secondary btn-sm inline-flex items-center gap-1.5 text-red-600" type="button" :disabled="changingUserId === item.user_id" @click="requestStatusChange(item, false)"><Icon name="xCircle" size="sm" aria-hidden="true" />{{ t('admin.studentRecharge.revoke') }}</button>
                  <button v-else class="btn btn-secondary btn-sm inline-flex items-center gap-1.5 text-emerald-700" type="button" :disabled="changingUserId === item.user_id" @click="requestStatusChange(item, true)"><Icon name="userPlus" size="sm" aria-hidden="true" />{{ t('admin.studentRecharge.grant') }}</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="space-y-4 px-5 py-5">
        <div class="flex items-center justify-between gap-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.studentRecharge.auditsTitle') }}</h3>
          <button class="btn-icon" type="button" :title="t('admin.studentRecharge.refreshAudits')" :aria-label="t('admin.studentRecharge.refreshAudits')" :disabled="auditsLoading" @click="loadAudits"><Icon name="refresh" size="sm" :class="auditsLoading ? 'animate-spin' : ''" aria-hidden="true" /></button>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full min-w-[900px]">
            <thead><tr><th class="table-th">{{ t('admin.studentRecharge.grantedAt') }}</th><th class="table-th">{{ t('admin.studentRecharge.account') }}</th><th class="table-th">{{ t('admin.studentRecharge.action') }}</th><th class="table-th">{{ t('admin.studentRecharge.administrator') }}</th><th class="table-th">{{ t('admin.studentRecharge.reason') }}</th><th class="table-th">{{ t('admin.studentRecharge.ip') }}</th></tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-if="auditsLoading"><td colspan="6" class="px-4 py-8 text-center text-sm text-gray-400">{{ t('admin.studentRecharge.loading') }}</td></tr>
              <tr v-else-if="!audits.length"><td colspan="6" class="px-4 py-8 text-center text-sm text-gray-400">{{ t('admin.studentRecharge.noAudits') }}</td></tr>
              <tr v-for="item in audits" :key="item.id">
                <td class="table-td">{{ formatDate(item.created_at) }}</td>
                <td class="table-td">{{ item.user_email }} <span class="text-xs text-gray-400">#{{ item.user_id }}</span></td>
                <td class="table-td">{{ item.action === 'grant' ? t('admin.studentRecharge.granted') : t('admin.studentRecharge.revoked') }}</td>
                <td class="table-td">{{ item.admin_email }} <span class="text-xs text-gray-400">#{{ item.admin_user_id }}</span></td>
                <td class="table-td">{{ item.reason || '-' }}</td>
                <td class="table-td">{{ item.client_ip || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>

    <ConfirmDialog
      :show="pendingAction !== null"
      :title="pendingAction?.student ? t('admin.studentRecharge.grantTitle') : t('admin.studentRecharge.revokeTitle')"
      :message="actionMessage"
      :confirm-text="pendingAction?.student ? t('admin.studentRecharge.grantConfirm') : t('admin.studentRecharge.revokeConfirm')"
      :danger="pendingAction?.student === false"
      @confirm="confirmStatusChange"
      @cancel="pendingAction = null"
    />
    <TotpStepUpDialog :controller="stepUp" />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpBlocked, isStepUpCancelled, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  getStudentRechargeBenefitConfig,
  grantStudentAccount,
  listStudentAccountAuditLogs,
  listStudentAccounts,
  revokeStudentAccount,
  updateStudentRechargeBenefitConfig,
  type StudentAccountAdminItem,
  type StudentAccountAuditItem,
  type StudentRechargeBenefitConfig,
} from '@/api/studentRechargeBenefit'

const { t, locale } = useI18n()
const appStore = useAppStore()
const stepUp = useStepUp()
const loading = ref(true)
const saving = ref(false)
const usersLoading = ref(false)
const auditsLoading = ref(false)
const searched = ref(false)
const keyword = ref('')
const users = ref<StudentAccountAdminItem[]>([])
const audits = ref<StudentAccountAuditItem[]>([])
const revokeReasons = reactive<Record<number, string>>({})
const changingUserId = ref<number | null>(null)
const pendingAction = ref<{ item: StudentAccountAdminItem; student: boolean } | null>(null)
const form = reactive<StudentRechargeBenefitConfig>({
  enabled: false,
  bonus_rate: 0.05,
  min_recharge_amount: 10,
  per_order_cap: 100,
})
let confirmedConfig: StudentRechargeBenefitConfig | null = null

const bonusPercent = computed({
  get: () => Number((form.bonus_rate * 100).toFixed(4)),
  set: (value: number) => { form.bonus_rate = Number(value) / 100 },
})

const previewBonus = computed(() => {
  const cap = Number(form.per_order_cap)
  const raw = 100 * Number(form.bonus_rate)
  if (!Number.isFinite(raw) || raw < 0) return 0
  return Number.isFinite(cap) && cap > 0 ? Math.min(raw, cap) : raw
})

const actionMessage = computed(() => {
  const action = pendingAction.value
  if (!action) return ''
  return t('admin.studentRecharge.confirmMessage', {
    action: action.student ? t('admin.studentRecharge.grant') : t('admin.studentRecharge.revoke'),
    email: action.item.email,
    id: action.item.user_id,
  })
})

function formatAmount(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return new Intl.NumberFormat(locale.value, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)
}

function formatDate(value: string): string {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString(locale.value)
}

function parseConfig(value: unknown): StudentRechargeBenefitConfig | null {
  if (!value || typeof value !== 'object') return null
  const record = value as Record<string, unknown>
  const bonusRate = Number(record.bonus_rate)
  const minimum = Number(record.min_recharge_amount)
  const cap = Number(record.per_order_cap)
  if (!Number.isFinite(bonusRate) || bonusRate <= 0 || bonusRate > 10) return null
  if (!Number.isFinite(minimum) || minimum < 0) return null
  if (!Number.isFinite(cap) || cap < 0) return null
  return { enabled: record.enabled === true, bonus_rate: bonusRate, min_recharge_amount: minimum, per_order_cap: cap }
}

function applyConfig(config: StudentRechargeBenefitConfig, remember = true): void {
  Object.assign(form, config)
  if (remember) confirmedConfig = { ...config }
}

function buildConfig(): StudentRechargeBenefitConfig | null {
  const config = {
    enabled: form.enabled === true,
    bonus_rate: Number(form.bonus_rate),
    min_recharge_amount: Number(form.min_recharge_amount),
    per_order_cap: Number(form.per_order_cap),
  }
  if (!Number.isFinite(config.bonus_rate) || config.bonus_rate <= 0 || config.bonus_rate > 10) {
    appStore.showError(t('admin.studentRecharge.invalidRate'))
    return null
  }
  if (!Number.isFinite(config.min_recharge_amount) || config.min_recharge_amount < 0) {
    appStore.showError(t('admin.studentRecharge.invalidMinimum'))
    return null
  }
  if (!Number.isFinite(config.per_order_cap) || config.per_order_cap < 0) {
    appStore.showError(t('admin.studentRecharge.invalidCap'))
    return null
  }
  return config
}

async function loadConfig(): Promise<void> {
  loading.value = true
  try {
    const config = parseConfig(await getStudentRechargeBenefitConfig())
    if (!config) throw new Error('Invalid student benefit configuration response')
    applyConfig(config)
  } catch (error: unknown) {
    form.enabled = false
    confirmedConfig = null
    appStore.showError(extractApiErrorMessage(error, t('admin.studentRecharge.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function saveConfig(): Promise<void> {
  if (saving.value) return
  const config = buildConfig()
  if (!config) return
  saving.value = true
  try {
    const saved = parseConfig(await stepUp.run(() => updateStudentRechargeBenefitConfig(config)))
    if (!saved) throw new Error('Invalid student benefit configuration response')
    applyConfig(saved)
    appStore.showSuccess(t('admin.studentRecharge.saved'))
  } catch (error: unknown) {
    if (confirmedConfig) applyConfig(confirmedConfig, false)
    if (isStepUpCancelled(error)) return
    if (isStepUpBlocked(error)) {
      appStore.showError(stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled'))
      return
    }
    appStore.showError(extractApiErrorMessage(error, t('admin.studentRecharge.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function searchUsers(): Promise<void> {
  usersLoading.value = true
  searched.value = true
  try {
    users.value = await listStudentAccounts(keyword.value)
  } catch (error: unknown) {
    users.value = []
    appStore.showError(extractApiErrorMessage(error, t('admin.studentRecharge.accountsFailed')))
  } finally {
    usersLoading.value = false
  }
}

async function loadAudits(): Promise<void> {
  auditsLoading.value = true
  try {
    audits.value = await listStudentAccountAuditLogs()
  } catch (error: unknown) {
    audits.value = []
    appStore.showError(extractApiErrorMessage(error, t('admin.studentRecharge.auditsFailed')))
  } finally {
    auditsLoading.value = false
  }
}

function requestStatusChange(item: StudentAccountAdminItem, student: boolean): void {
  pendingAction.value = { item, student }
}

async function confirmStatusChange(): Promise<void> {
  const action = pendingAction.value
  if (!action || changingUserId.value !== null) return
  pendingAction.value = null
  changingUserId.value = action.item.user_id
  try {
    if (action.student) {
      await stepUp.run(() => grantStudentAccount(action.item.user_id))
    } else {
      await stepUp.run(() => revokeStudentAccount(action.item.user_id, revokeReasons[action.item.user_id] || ''))
    }
    appStore.showSuccess(action.student ? t('admin.studentRecharge.grantSuccess') : t('admin.studentRecharge.revokeSuccess'))
    await Promise.all([searchUsers(), loadAudits()])
  } catch (error: unknown) {
    if (isStepUpCancelled(error)) return
    if (isStepUpBlocked(error)) {
      appStore.showError(stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled'))
      return
    }
    appStore.showError(extractApiErrorMessage(error, action.student ? t('admin.studentRecharge.grantFailed') : t('admin.studentRecharge.revokeFailed')))
  } finally {
    changingUserId.value = null
  }
}

onMounted(() => {
  void loadConfig()
  void loadAudits()
})
</script>

<style scoped>
.table-th {
  @apply whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400;
}

.table-td {
  @apply px-4 py-3 text-sm text-gray-600 dark:text-dark-300;
}
</style>
