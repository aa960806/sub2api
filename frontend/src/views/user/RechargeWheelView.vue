<template>
  <AppLayout>
    <section class="mx-auto max-w-6xl space-y-6" data-testid="recharge-wheel-page">
      <header class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div class="mb-2 flex items-center gap-2 text-cyan-700 dark:text-cyan-400">
            <Icon name="creditCard" size="md" aria-hidden="true" />
            <span class="text-xs font-semibold uppercase">{{ t('inviteActivities.rechargeWheel.title') }}</span>
          </div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.rechargeWheel.title') }}</h1>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-dark-400">{{ t('inviteActivities.rechargeWheel.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading || claiming" @click="loadStatus">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
          <span>{{ t('inviteActivities.common.refresh') }}</span>
        </button>
      </header>

      <div v-if="loading" class="rounded-lg border border-gray-200 bg-white px-6 py-16 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400" aria-busy="true">
        {{ t('inviteActivities.common.loading') }}
      </div>

      <div v-else-if="disabled" class="rounded-lg border border-dashed border-gray-300 bg-white px-6 py-16 text-center dark:border-dark-600 dark:bg-dark-900" data-testid="recharge-wheel-disabled">
        <Icon name="lock" size="xl" class="mx-auto text-gray-300 dark:text-dark-600" aria-hidden="true" />
        <p class="mt-4 text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('inviteActivities.common.disabled') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('inviteActivities.common.disabledHint') }}</p>
      </div>

      <template v-else-if="status">
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <article class="metric-block"><span>{{ t('inviteActivities.rechargeWheel.rechargeProgress') }}</span><strong>${{ money(status.recharged_amount) }}</strong></article>
          <article class="metric-block"><span>{{ t('inviteActivities.rechargeWheel.threshold') }}</span><strong>${{ money(status.threshold) }}</strong></article>
          <article class="metric-block"><span>{{ t('inviteActivities.rechargeWheel.remaining') }}</span><strong>{{ status.remaining_chances }}</strong></article>
          <article class="metric-block"><span>{{ t('inviteActivities.rechargeWheel.used') }}</span><strong>{{ status.used_chances }}</strong></article>
        </div>

        <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
          <section class="wheel-stage" :class="{ 'wheel-stage--claiming': claiming }">
            <div class="grid gap-6 sm:grid-cols-2">
              <div>
                <h2 class="mb-3 text-center text-sm font-semibold text-gray-700 dark:text-dark-200">{{ t('inviteActivities.rechargeWheel.amountPool') }}</h2>
                <div class="wheel-grid">
                  <span
                    v-for="(item, index) in status.amounts"
                    :key="`amount-${item.amount}-${index}`"
                    class="wheel-segment"
                    :class="{ 'wheel-segment--selected': lastResult?.amount_index === index }"
                  >${{ money(item.amount) }}</span>
                </div>
              </div>
              <div>
                <h2 class="mb-3 text-center text-sm font-semibold text-gray-700 dark:text-dark-200">{{ t('inviteActivities.rechargeWheel.multiplierPool') }}</h2>
                <div class="wheel-grid wheel-grid--multiplier">
                  <span
                    v-for="(item, index) in status.multipliers"
                    :key="`multiplier-${item.multiplier}-${index}`"
                    class="wheel-segment"
                    :class="{ 'wheel-segment--selected': lastResult?.multiplier_index === index }"
                  >×{{ compactNumber(item.multiplier) }}</span>
                </div>
              </div>
            </div>
          </section>

          <aside class="space-y-4">
            <div v-if="lastResult" class="rounded-lg border border-emerald-200 bg-emerald-50 p-4 dark:border-emerald-900/60 dark:bg-emerald-950/30" role="status" data-testid="recharge-wheel-result">
              <p class="text-xs font-semibold uppercase text-emerald-700 dark:text-emerald-400">{{ t('inviteActivities.rechargeWheel.resultTitle') }}</p>
              <p class="mt-2 text-xl font-semibold tabular-nums text-emerald-800 dark:text-emerald-300">
                {{ t('inviteActivities.rechargeWheel.result', { amount: money(lastResult.amount), multiplier: compactNumber(lastResult.multiplier), total: money(lastResult.total) }) }}
              </p>
            </div>
            <button
              type="button"
              class="btn btn-primary inline-flex min-h-12 w-full items-center justify-center gap-2"
              :disabled="!status.can_claim || claiming"
              data-testid="recharge-wheel-claim"
              @click="claim"
            >
              <Icon name="refresh" size="sm" :class="claiming ? 'animate-spin' : ''" aria-hidden="true" />
              <span>{{ claiming ? t('inviteActivities.common.claiming') : status.can_claim ? t('inviteActivities.rechargeWheel.draw') : t('inviteActivities.rechargeWheel.noChance') }}</span>
            </button>
            <button type="button" class="btn btn-secondary inline-flex w-full items-center justify-center gap-2" @click="goRecharge">
              <Icon name="creditCard" size="sm" aria-hidden="true" />
              <span>{{ t('inviteActivities.rechargeWheel.recharge') }}</span>
            </button>
          </aside>
        </div>
      </template>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  claimRechargeWheel,
  getRechargeWheelStatus,
  type RechargeWheelResult,
  type RechargeWheelStatus,
} from '@/api/inviteActivities'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isInviteActivitySettingsEnabled } from '@/utils/inviteActivities'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const status = ref<RechargeWheelStatus | null>(null)
const lastResult = ref<RechargeWheelResult | null>(null)
const loading = ref(false)
const claiming = ref(false)
const featureDisabled = ref(false)
const disabled = computed(() => featureDisabled.value || status.value?.enabled === false)

function money(value: number): string { return Number(value || 0).toFixed(2) }
function compactNumber(value: number): string { return Number(value || 0).toLocaleString(undefined, { maximumFractionDigits: 2 }) }

async function publicFlagEnabled(): Promise<boolean> {
  if (!appStore.publicSettingsLoaded) await appStore.fetchPublicSettings()
  return isInviteActivitySettingsEnabled(
    appStore.publicSettingsLoaded,
    appStore.cachedPublicSettings,
    'subnexus_recharge_wheel_enabled',
  )
}

async function loadStatus(): Promise<void> {
  loading.value = true
  try {
    if (!(await publicFlagEnabled())) {
      featureDisabled.value = true
      status.value = null
      return
    }
    const result = await getRechargeWheelStatus()
    status.value = result
    featureDisabled.value = !result.enabled
  } catch (error) {
    status.value = null
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function claim(): Promise<void> {
  if (!status.value?.can_claim || claiming.value) return
  claiming.value = true
  // Re-check before the balance-affecting POST in case the administrator
  // disabled this activity while the page was still open.
  let enabled = false
  try {
    enabled = await publicFlagEnabled()
  } catch {
    enabled = false
  }
  if (!enabled) {
    featureDisabled.value = true
    status.value = null
    claiming.value = false
    return
  }
  lastResult.value = null
  try {
    const result = await claimRechargeWheel()
    status.value = result
    if (result.result) {
      lastResult.value = result.result
      appStore.showSuccess(t('inviteActivities.rechargeWheel.won', {
        amount: money(result.result.amount),
        multiplier: compactNumber(result.result.multiplier),
        total: money(result.result.total),
      }))
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.claimFailed')))
    await loadStatus()
  } finally {
    claiming.value = false
  }
}

function goRecharge(): void { void router.push('/purchase') }
onMounted(() => { void loadStatus() })
</script>

<style scoped>
.metric-block { @apply rounded-lg border border-gray-200 bg-white px-4 py-4 dark:border-dark-700 dark:bg-dark-900; }
.metric-block span { @apply block text-xs font-medium text-gray-500 dark:text-dark-400; }
.metric-block strong { @apply mt-2 block text-2xl font-semibold tabular-nums text-gray-900 dark:text-white; }
.wheel-stage { @apply rounded-lg border border-gray-200 bg-gray-50 p-5 transition-opacity dark:border-dark-700 dark:bg-dark-900/60; }
.wheel-stage--claiming { @apply opacity-70; }
.wheel-grid { @apply grid min-h-56 grid-cols-2 gap-3 rounded-full border-8 border-white bg-cyan-50 p-6 shadow-inner dark:border-dark-800 dark:bg-cyan-950/20; }
.wheel-grid--multiplier { @apply bg-amber-50 dark:bg-amber-950/20; }
.wheel-segment { @apply flex min-h-16 items-center justify-center rounded-lg border border-gray-200 bg-white px-2 text-center text-sm font-semibold tabular-nums text-gray-700 transition-colors dark:border-dark-700 dark:bg-dark-900 dark:text-dark-200; }
.wheel-segment--selected { @apply border-emerald-500 bg-emerald-100 text-emerald-800 ring-2 ring-emerald-500/20 dark:border-emerald-500 dark:bg-emerald-950/40 dark:text-emerald-300; }
</style>
