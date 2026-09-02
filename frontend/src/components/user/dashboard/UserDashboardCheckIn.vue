<template>
  <section v-if="featureEnabled && !hidden" class="card p-5" data-testid="dashboard-checkin">
    <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div class="min-w-0">
        <p class="text-sm font-medium text-gray-500 dark:text-dark-400">{{ t('checkin.title') }}</p>
        <div class="mt-1 flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <h2 class="text-xl font-bold text-gray-900 dark:text-white">{{ statusText }}</h2>
          <span v-if="status?.enabled" class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('checkin.continuous', { count: continuousStreak }) }}
          </span>
        </div>
      </div>
      <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
        <Icon name="gift" size="md" aria-hidden="true" />
      </div>
    </div>

    <div v-if="loading" class="mt-5 grid grid-cols-3 gap-2 sm:grid-cols-5 lg:grid-cols-[repeat(auto-fit,minmax(68px,1fr))]" aria-busy="true">
      <div v-for="day in 7" :key="`checkin-loading-${day}`" class="h-24 animate-pulse rounded-md bg-gray-100 dark:bg-dark-800" />
    </div>

    <div v-else-if="status?.enabled" class="mt-5 grid grid-cols-3 gap-2 sm:grid-cols-5 lg:grid-cols-[repeat(auto-fit,minmax(68px,1fr))]">
      <button
        v-for="day in cycleDays"
        :key="`checkin-day-${day}`"
        type="button"
        class="relative flex h-24 min-w-0 flex-col items-center justify-center gap-1 overflow-hidden rounded-md border px-1.5 transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-dark-900"
        :class="dayButtonClass(day)"
        :disabled="!isClaimableDay(day) || claiming"
        :title="dayButtonTitle(day)"
        @click="claim"
      >
        <span class="text-[11px] font-medium">{{ t('checkin.day', { day }) }}</span>
        <span v-if="day === milestoneDays" class="absolute right-1.5 top-1.5 rounded bg-amber-500 px-1.5 py-0.5 text-[9px] font-bold text-white shadow-sm">{{ t('checkin.milestone') }}</span>
        <span class="relative flex h-8 w-8 items-center justify-center rounded-full" :class="dayIconClass(day)">
          <Icon v-if="isClaimableDay(day) && claiming" name="refresh" size="sm" class="animate-spin" />
          <Icon v-else-if="day === milestoneDays" name="gift" size="sm" :stroke-width="2.3" />
          <Icon v-else-if="isCompletedDay(day)" name="check" size="sm" :stroke-width="2.5" />
          <Icon v-else-if="isClaimableDay(day)" name="gift" size="sm" />
          <Icon v-else name="lock" size="sm" />
          <span v-if="day === milestoneDays && isCompletedDay(day)" class="absolute -bottom-1 -right-1 flex h-4 w-4 items-center justify-center rounded-full bg-emerald-500 text-white ring-2 ring-amber-50 dark:ring-amber-950">
            <Icon name="check" size="xs" :stroke-width="3" aria-hidden="true" />
          </span>
        </span>
        <span class="max-w-full truncate text-[10px] font-semibold">{{ day === milestoneDays ? t('checkin.milestone') : t('checkin.daily') }}</span>
      </button>
    </div>

    <div v-if="frozenAmount > 0" class="mt-4 flex items-center gap-2 rounded-md bg-indigo-50 px-3 py-2 text-xs text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-300">
      <Icon name="gift" size="sm" aria-hidden="true" />
      <span>{{ t('checkin.frozen', { amount: formatMoney(frozenAmount) }) }}</span>
    </div>

    <button v-if="limitReached" type="button" class="btn btn-primary mt-4 w-full" @click="goRecharge">
      {{ t('checkin.recharge') }}
    </button>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { claimCheckIn, getCheckInStatus, type CheckInStatus } from '@/api/checkin'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { isCheckInEnabled } from '@/utils/featureFlags'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const router = useRouter()
const status = ref<CheckInStatus | null>(null)
const loading = ref(false)
const claiming = ref(false)
const loadedForEnabledState = ref(false)

const featureEnabled = computed(() => isCheckInEnabled())
const hidden = computed(() => status.value?.locked === true)
const limitReached = computed(() => status.value?.limit_reached === true && !status.value?.checked_in)
const frozenAmount = computed(() => Number(status.value?.frozen_amount || 0))
const continuousStreak = computed(() => Math.max(0, Number(status.value?.continuous_streak || 0)))
const milestoneDays = computed(() => Math.min(15, Math.max(1, Number(status.value?.milestone_days || 7))))
const cycleDays = computed(() => Array.from({ length: milestoneDays.value }, (_, index) => index + 1))
const cycleDay = computed(() => Math.min(milestoneDays.value, Math.max(1, Number(status.value?.cycle_day || 1))))
const completedDays = computed(() => Math.min(milestoneDays.value, Math.max(0, Number(status.value?.cycle_completed_days || 0))))

const statusText = computed(() => {
  if (loading.value) return t('common.loading')
  if (!status.value?.enabled) return t('checkin.disabled')
  if (status.value.checked_in && status.value.today_frozen) return t('checkin.frozenToday')
  return status.value.checked_in ? t('checkin.claimed') : t('checkin.pending')
})

function isCompletedDay(day: number): boolean {
  return day <= completedDays.value
}

function isClaimableDay(day: number): boolean {
  return Boolean(status.value?.enabled && !status.value.checked_in && !limitReached.value && day === cycleDay.value)
}

function dayButtonClass(day: number): string {
  if (day === milestoneDays.value) {
    if (isCompletedDay(day)) return 'border-amber-400 bg-amber-50 text-amber-800 shadow-sm dark:border-amber-600 dark:bg-amber-950/40 dark:text-amber-200'
    if (isClaimableDay(day)) return 'cursor-pointer border-amber-400 bg-amber-50 text-amber-800 shadow-sm hover:border-amber-500 hover:bg-amber-100 dark:border-amber-600 dark:bg-amber-950/40 dark:text-amber-200'
    return 'cursor-not-allowed border-amber-200 bg-amber-50/60 text-amber-600 dark:border-amber-900/70 dark:bg-amber-950/20 dark:text-amber-500'
  }
  if (isCompletedDay(day)) return 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300'
  if (isClaimableDay(day)) return 'cursor-pointer border-primary-400 bg-primary-50 text-primary-700 hover:border-primary-500 hover:bg-primary-100 dark:border-primary-600 dark:bg-primary-950/30 dark:text-primary-300'
  return 'cursor-not-allowed border-gray-200 bg-gray-50 text-gray-400 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-500'
}

function dayIconClass(day: number): string {
  if (day === milestoneDays.value) return 'bg-amber-200 text-amber-800 shadow-inner dark:bg-amber-800/50 dark:text-amber-200'
  if (isCompletedDay(day)) return 'bg-emerald-200/70 dark:bg-emerald-800/40'
  if (isClaimableDay(day)) return 'bg-primary-200/70 dark:bg-primary-800/40'
  return 'bg-gray-200/70 dark:bg-dark-700'
}

function dayButtonTitle(day: number): string {
  if (day === milestoneDays.value) {
    if (isCompletedDay(day)) return t('checkin.milestoneDone', { day })
    if (isClaimableDay(day)) return t('checkin.claimMilestone', { day })
    return t('checkin.futureMilestone', { day })
  }
  if (isCompletedDay(day)) return t('checkin.dayDone', { day })
  if (isClaimableDay(day)) return t('checkin.claimDay', { day })
  return t('checkin.futureDay', { day })
}

async function loadStatus(): Promise<void> {
  if (!featureEnabled.value || loadedForEnabledState.value) return
  loadedForEnabledState.value = true
  loading.value = true
  try {
    status.value = await getCheckInStatus()
  } catch {
    status.value = null
  } finally {
    loading.value = false
  }
}

async function ensureFeatureAndLoad(): Promise<void> {
  if (!appStore.publicSettingsLoaded) {
    await appStore.fetchPublicSettings()
  }
  await loadStatus()
}

async function claim(): Promise<void> {
  // Re-check the independent rollout switch immediately before the balance
  // writing request. A dashboard can remain open while an administrator turns
  // the feature off; the backend is authoritative, but the stale page should
  // not issue a known-disabled POST.
  if (!featureEnabled.value || claiming.value || !isClaimableDay(cycleDay.value)) return
  claiming.value = true
  try {
    status.value = await claimCheckIn()
    await authStore.refreshUser()
    appStore.showSuccess(status.value.today_frozen ? t('checkin.claimedFrozen') : t('checkin.claimedAmount', { amount: formatMoney(status.value.amount || 0) }))
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : t('checkin.claimFailed')
    appStore.showError(message)
    loadedForEnabledState.value = false
    await loadStatus()
  } finally {
    claiming.value = false
  }
}

function goRecharge(): void {
  void router.push('/purchase')
}

function formatMoney(value: number): string {
  return Number(value || 0).toFixed(2)
}

watch(() => appStore.cachedPublicSettings?.subnexus_checkin_enabled, (enabled) => {
  if (enabled === true) void loadStatus()
})

onMounted(() => { void ensureFeatureAndLoad() })
</script>
