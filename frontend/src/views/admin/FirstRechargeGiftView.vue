<template>
  <AppLayout>
    <section class="mx-auto max-w-4xl space-y-6" data-testid="admin-first-recharge-gift-page">
      <header class="border-b border-gray-200 pb-5 dark:border-dark-700">
        <div class="flex items-center gap-2 text-primary-600 dark:text-primary-400">
          <Icon name="gift" size="md" aria-hidden="true" />
          <span class="text-xs font-semibold uppercase">{{ t('nav.firstRechargeGift') }}</span>
        </div>
        <h1 class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">
          {{ t('admin.firstRechargeGift.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.firstRechargeGift.description') }}
        </p>
      </header>

      <div v-if="loading" class="flex items-center justify-center py-16" aria-busy="true">
        <Icon name="refresh" size="lg" class="animate-spin text-primary-500" aria-hidden="true" />
      </div>

      <div
        v-else-if="loadFailed"
        class="rounded-md border border-red-200 bg-red-50 px-5 py-4 text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
        role="alert"
        data-testid="first-recharge-load-error"
      >
        <div class="flex items-start gap-3">
          <Icon name="exclamationTriangle" size="md" class="mt-0.5 shrink-0" aria-hidden="true" />
          <div class="min-w-0 flex-1">
            <p class="text-sm font-medium">{{ t('admin.firstRechargeGift.loadFailed') }}</p>
            <button
              type="button"
              class="btn btn-secondary btn-sm mt-3 inline-flex items-center gap-2"
              data-testid="first-recharge-retry"
              @click="loadConfig"
            >
              <Icon name="refresh" size="sm" aria-hidden="true" />
              <span>{{ t('admin.firstRechargeGift.retry') }}</span>
            </button>
          </div>
        </div>
      </div>

      <form v-else class="space-y-6" @submit.prevent="saveConfig">
        <section class="card p-6">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">
                {{ t('admin.firstRechargeGift.switchTitle') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
                {{ t('admin.firstRechargeGift.switchHint') }}
              </p>
            </div>
            <Toggle v-model="form.enabled" data-testid="first-recharge-enabled" />
          </div>
          <p
            v-if="!form.enabled"
            class="mt-4 rounded-md border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400"
          >
            {{ t('admin.firstRechargeGift.disabledHint') }}
          </p>
        </section>

        <section class="card space-y-5 p-6">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.firstRechargeGift.offerTitle') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.firstRechargeGift.offerHint') }}
            </p>
          </div>

          <div class="grid gap-4 sm:grid-cols-2">
            <label class="space-y-1.5">
              <span class="input-label">{{ t('admin.firstRechargeGift.price') }}</span>
              <input
                v-model.number="form.price"
                class="input"
                data-testid="first-recharge-price"
                type="number"
                min="0.01"
                max="1000000"
                step="0.01"
                inputmode="decimal"
                required
              />
              <span class="input-hint">{{ t('admin.firstRechargeGift.priceHint') }}</span>
            </label>
            <label class="space-y-1.5">
              <span class="input-label">{{ t('admin.firstRechargeGift.creditedAmount') }}</span>
              <input
                v-model.number="form.credited_amount"
                class="input"
                data-testid="first-recharge-credit"
                type="number"
                min="0.01"
                max="1000000000"
                step="0.01"
                inputmode="decimal"
                required
              />
              <span class="input-hint">{{ t('admin.firstRechargeGift.creditedAmountHint') }}</span>
            </label>
          </div>

          <div class="rounded-md border border-gray-200 px-4 py-3 dark:border-dark-700">
            <div class="flex items-center justify-between gap-4">
              <span class="text-sm text-gray-600 dark:text-dark-300">
                {{ t('admin.firstRechargeGift.ratio') }}
              </span>
              <output class="text-sm font-semibold text-gray-900 dark:text-white" data-testid="first-recharge-ratio">
                {{ displayRatio }}
              </output>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.firstRechargeGift.ratioHint') }}
            </p>
          </div>
        </section>

        <div class="flex justify-end">
          <button
            type="submit"
            class="btn btn-primary inline-flex items-center gap-2"
            :disabled="saving"
            data-testid="first-recharge-save"
          >
            <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="saving ? 'animate-spin' : ''" aria-hidden="true" />
            <span>{{ saving ? t('common.saving') : t('common.save') }}</span>
          </button>
        </div>
      </form>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  adminPaymentAPI,
  type AdminFirstRechargeGiftConfig,
} from '@/api/admin/payment'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const loadFailed = ref(false)
const saving = ref(false)

const form = reactive<AdminFirstRechargeGiftConfig>({
  enabled: false,
  price: 9.9,
  credited_amount: 12,
  ratio: 1.2,
})
let confirmedConfig: AdminFirstRechargeGiftConfig | null = null

function round(value: number, precision: number): number {
  const factor = 10 ** precision
  return Math.round((value + Number.EPSILON) * factor) / factor
}

function parseConfig(value: unknown): AdminFirstRechargeGiftConfig | null {
  if (!value || typeof value !== 'object') return null
  const record = value as Record<string, unknown>
  const price = Number(record.price)
  const creditedAmount = Number(record.credited_amount)
  const ratio = Number(record.ratio)
  if (!Number.isFinite(price) || price <= 0 || price > 1_000_000) return null
  if (!Number.isFinite(creditedAmount) || creditedAmount <= 0 || creditedAmount > 1_000_000_000) return null
  if (!Number.isFinite(ratio) || ratio <= 0 || ratio > 1_000_000) return null
  return {
    enabled: record.enabled === true,
    price: round(price, 2),
    credited_amount: round(creditedAmount, 2),
    ratio: round(ratio, 4),
  }
}

function applyConfig(config: AdminFirstRechargeGiftConfig, remember = true): void {
  Object.assign(form, config)
  if (remember) confirmedConfig = { ...config }
}

const calculatedRatio = computed(() => {
  const price = Number(form.price)
  const creditedAmount = Number(form.credited_amount)
  if (!Number.isFinite(price) || price <= 0 || !Number.isFinite(creditedAmount) || creditedAmount <= 0) return 0
  return round(creditedAmount / price, 4)
})

const displayRatio = computed(() => calculatedRatio.value > 0 ? `x${calculatedRatio.value.toFixed(4)}` : '-')

function buildConfig(): AdminFirstRechargeGiftConfig | null {
  const price = round(Number(form.price), 2)
  const creditedAmount = round(Number(form.credited_amount), 2)
  if (!Number.isFinite(price) || price <= 0 || price > 1_000_000) {
    appStore.showError(t('admin.firstRechargeGift.invalidPrice'))
    return null
  }
  if (!Number.isFinite(creditedAmount) || creditedAmount <= 0 || creditedAmount > 1_000_000_000) {
    appStore.showError(t('admin.firstRechargeGift.invalidCreditedAmount'))
    return null
  }
  const ratio = round(creditedAmount / price, 4)
  if (!Number.isFinite(ratio) || ratio <= 0 || ratio > 1_000_000) {
    appStore.showError(t('admin.firstRechargeGift.invalidRatio'))
    return null
  }
  return {
    enabled: form.enabled === true,
    price,
    credited_amount: creditedAmount,
    ratio,
  }
}

async function loadConfig(): Promise<void> {
  loading.value = true
  loadFailed.value = false
  try {
    const response = await adminPaymentAPI.getFirstRechargeGiftConfig()
    const config = parseConfig(response.data)
    if (!config) throw new Error('Invalid first-recharge configuration response')
    applyConfig(config)
  } catch (error) {
    loadFailed.value = true
    confirmedConfig = null
    form.enabled = false
    appStore.showError(extractApiErrorMessage(error, t('admin.firstRechargeGift.loadFailed')))
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
    const response = await adminPaymentAPI.updateFirstRechargeGiftConfig(config)
    const saved = parseConfig(response.data)
    if (!saved) throw new Error('Invalid first-recharge configuration response')
    applyConfig(saved)
    appStore.showSuccess(t('admin.firstRechargeGift.saved'))
  } catch (error) {
    if (confirmedConfig) applyConfig(confirmedConfig, false)
    appStore.showError(extractApiErrorMessage(error, t('admin.firstRechargeGift.saveFailed')))
  } finally {
    saving.value = false
  }
}

onMounted(() => { void loadConfig() })
</script>
