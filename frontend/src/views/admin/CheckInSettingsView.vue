<template>
  <AppLayout>
    <section class="mx-auto max-w-5xl space-y-6" data-testid="admin-checkin-page">
      <header class="border-b border-gray-200 pb-5 dark:border-dark-700">
        <div class="flex items-center gap-2 text-primary-600 dark:text-primary-400">
          <Icon name="calendar" size="md" aria-hidden="true" />
          <span class="text-xs font-semibold uppercase">{{ t('nav.checkin') }}</span>
        </div>
        <h1 class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.checkin.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.checkin.description') }}</p>
      </header>

      <div v-if="loading" class="flex items-center justify-center py-16" aria-busy="true">
        <Icon name="refresh" size="lg" class="animate-spin text-primary-500" aria-hidden="true" />
      </div>

      <div
        v-else-if="loadFailed"
        class="rounded-md border border-red-200 bg-red-50 px-5 py-4 text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
        role="alert"
        data-testid="checkin-load-error"
      >
        <p class="text-sm font-medium">{{ t('admin.checkin.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary btn-sm mt-3 inline-flex items-center gap-2" data-testid="checkin-retry" @click="loadConfig">
          <Icon name="refresh" size="sm" aria-hidden="true" />
          <span>{{ t('admin.checkin.retry') }}</span>
        </button>
      </div>

      <form v-else class="space-y-6" @submit.prevent="saveConfig">
        <section class="card p-6">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.checkin.switchTitle') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.checkin.switchHint') }}</p>
            </div>
            <Toggle v-model="form.enabled" data-testid="checkin-enabled" />
          </div>
          <p v-if="!form.enabled" class="mt-4 rounded-md border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">{{ t('admin.checkin.disabledHint') }}</p>
        </section>

        <section class="card space-y-5 p-6">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.checkin.rewardTitle') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.checkin.rewardHint') }}</p>
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <label class="space-y-1.5">
              <span class="input-label">{{ t('admin.checkin.cycleMode') }}</span>
              <select v-model="form.checkin_cycle_mode" class="input">
                <option value="reset">{{ t('admin.checkin.cycleReset') }}</option>
                <option value="cumulative">{{ t('admin.checkin.cycleCumulative') }}</option>
              </select>
            </label>
            <label class="space-y-1.5">
              <span class="input-label">{{ t('admin.checkin.milestoneDays') }}</span>
              <input v-model.number="form.checkin_milestone_days" class="input" type="number" min="1" max="15" step="1" inputmode="numeric" />
            </label>
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <label class="space-y-1.5">
              <span class="input-label">{{ t('admin.checkin.dailyMin') }}</span>
              <input v-model.number="form.checkin_min" class="input" type="number" min="0" step="0.01" inputmode="decimal" />
            </label>
            <label class="space-y-1.5">
              <span class="input-label">{{ t('admin.checkin.dailyMax') }}</span>
              <input v-model.number="form.checkin_max" class="input" type="number" min="0.02" step="0.01" inputmode="decimal" />
            </label>
            <label class="space-y-1.5">
              <span class="input-label">{{ t('admin.checkin.milestoneMin') }}</span>
              <input v-model.number="form.checkin_milestone_min" class="input" type="number" min="0" step="0.01" inputmode="decimal" />
            </label>
            <label class="space-y-1.5">
              <span class="input-label">{{ t('admin.checkin.milestoneMax') }}</span>
              <input v-model.number="form.checkin_milestone_max" class="input" type="number" min="0.02" step="0.01" inputmode="decimal" />
            </label>
          </div>
          <label class="flex items-center justify-between gap-3 border-t border-gray-100 pt-4 text-sm text-gray-700 dark:border-dark-700 dark:text-dark-200">
            <span>{{ t('admin.checkin.ipLimit') }}</span>
            <Toggle v-model="form.checkin_ip_limit" data-testid="checkin-ip-limit" />
          </label>
        </section>

        <section class="card space-y-5 p-6">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.checkin.paidTitle') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.checkin.paidHint') }}</p>
          </div>
          <label class="space-y-1.5">
            <span class="input-label">{{ t('admin.checkin.paidMode') }}</span>
            <select v-model="form.checkin_paid_mode" class="input">
              <option value="off">{{ t('admin.checkin.paidOff') }}</option>
              <option value="limit">{{ t('admin.checkin.paidLimit') }}</option>
              <option value="hide">{{ t('admin.checkin.paidHide') }}</option>
            </select>
          </label>
          <div v-if="form.checkin_paid_mode === 'limit'" class="space-y-4 rounded-md border border-gray-200 p-4 dark:border-dark-700">
            <div class="grid gap-4 sm:grid-cols-2">
              <label class="space-y-1.5">
                <span class="input-label">{{ t('admin.checkin.freeMaxCount') }}</span>
                <input v-model.number="form.checkin_free_max_count" class="input" type="number" min="0" step="1" inputmode="numeric" />
              </label>
              <label class="space-y-1.5">
                <span class="input-label">{{ t('admin.checkin.freeMaxAmount') }}</span>
                <input v-model.number="form.checkin_free_max_amount" class="input" type="number" min="0" step="0.01" inputmode="decimal" />
              </label>
            </div>
            <label class="space-y-1.5">
              <span class="input-label">{{ t('admin.checkin.overLimitAction') }}</span>
              <select v-model="form.checkin_over_limit_action" class="input">
                <option value="prompt">{{ t('admin.checkin.overLimitPrompt') }}</option>
                <option value="freeze">{{ t('admin.checkin.overLimitFreeze') }}</option>
              </select>
            </label>
          </div>
        </section>

        <div class="flex items-center justify-end gap-3">
          <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.checkin.saveHint') }}</span>
          <button type="submit" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" data-testid="checkin-save">
            <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="saving ? 'animate-spin' : ''" aria-hidden="true" />
            <span>{{ saving ? t('admin.checkin.saving') : t('common.save') }}</span>
          </button>
        </div>
      </form>
  </section>
    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { getCheckInConfig, updateCheckInConfig, type CheckInConfig } from '@/api/checkin'
import { isStepUpBlocked, isStepUpCancelled, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const stepUp = useStepUp()
const loading = ref(true)
const loadFailed = ref(false)
const saving = ref(false)

const form = reactive<CheckInConfig>({
  enabled: false,
  checkin_ip_limit: false,
  checkin_min: 0.01,
  checkin_max: 0.1,
  checkin_cycle_mode: 'reset',
  checkin_milestone_days: 7,
  checkin_milestone_min: 0.1,
  checkin_milestone_max: 0.5,
  checkin_paid_mode: 'off',
  checkin_free_max_count: 0,
  checkin_free_max_amount: 0,
  checkin_over_limit_action: 'prompt',
})

function syncPublicFlag(enabled: boolean): void {
  const current = appStore.cachedPublicSettings
  if (current) appStore.cachedPublicSettings = { ...current, subnexus_checkin_enabled: enabled }
  try { window.dispatchEvent(new CustomEvent('checkin-config-changed')) } catch { /* test/non-browser runtime */ }
}

function applyConfig(config: CheckInConfig): void {
  Object.assign(form, config)
  syncPublicFlag(form.enabled)
}

async function loadConfig(): Promise<void> {
  loading.value = true
  loadFailed.value = false
  try {
    applyConfig(await getCheckInConfig())
  } catch (error) {
    loadFailed.value = true
    syncPublicFlag(false)
    appStore.showError(extractApiErrorMessage(error, t('admin.checkin.loadFailed')))
  } finally {
    loading.value = false
  }
}

function buildConfig(): CheckInConfig | null {
  const config: CheckInConfig = {
    enabled: form.enabled === true,
    checkin_ip_limit: form.checkin_ip_limit === true,
    checkin_min: Number(form.checkin_min),
    checkin_max: Number(form.checkin_max),
    checkin_cycle_mode: form.checkin_cycle_mode,
    checkin_milestone_days: Math.trunc(Number(form.checkin_milestone_days)),
    checkin_milestone_min: Number(form.checkin_milestone_min),
    checkin_milestone_max: Number(form.checkin_milestone_max),
    checkin_paid_mode: form.checkin_paid_mode,
    checkin_free_max_count: Math.trunc(Number(form.checkin_free_max_count)),
    checkin_free_max_amount: Number(form.checkin_free_max_amount),
    checkin_over_limit_action: form.checkin_over_limit_action,
  }
  const numeric = [config.checkin_min, config.checkin_max, config.checkin_milestone_min, config.checkin_milestone_max, config.checkin_free_max_amount]
  if (numeric.some((value) => !Number.isFinite(value)) || !Number.isFinite(config.checkin_milestone_days) || !Number.isFinite(config.checkin_free_max_count)) {
    appStore.showError(t('admin.checkin.invalidValues'))
    return null
  }
  return config
}

async function saveConfig(): Promise<void> {
  if (saving.value) return
  const config = buildConfig()
  if (!config) return
  saving.value = true
  try {
    applyConfig(await stepUp.run(() => updateCheckInConfig(config)))
    appStore.showSuccess(t('admin.checkin.saved'))
  } catch (error) {
    if (isStepUpCancelled(error)) return
    if (isStepUpBlocked(error)) {
      appStore.showError(stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled'))
      return
    }
    appStore.showError(extractApiErrorMessage(error, t('admin.checkin.saveFailed')))
  } finally {
    saving.value = false
  }
}

onMounted(() => { void loadConfig() })
</script>
