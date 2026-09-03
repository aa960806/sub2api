<template>
  <AppLayout>
    <section class="mx-auto max-w-4xl space-y-6" data-testid="admin-leaderboard-page">
      <header class="border-b border-gray-200 pb-5 dark:border-dark-700">
        <div class="flex items-center gap-2 text-primary-600 dark:text-primary-400">
          <Icon name="trophy" size="md" aria-hidden="true" />
          <span class="text-xs font-semibold uppercase">{{ t('nav.leaderboard') }}</span>
        </div>
        <h1 class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.leaderboard.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.leaderboard.description') }}</p>
      </header>

      <div v-if="loading" class="flex items-center justify-center py-16" aria-busy="true">
        <Icon name="refresh" size="lg" class="animate-spin text-primary-500" aria-hidden="true" />
      </div>

      <div
        v-else-if="loadFailed"
        class="rounded-md border border-red-200 bg-red-50 px-5 py-4 text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
        role="alert"
        data-testid="leaderboard-load-error"
      >
        <div class="flex items-start gap-3">
          <Icon name="exclamationTriangle" size="md" class="mt-0.5 shrink-0" aria-hidden="true" />
          <div class="min-w-0 flex-1">
            <p class="text-sm font-medium">{{ t('admin.leaderboard.loadFailed') }}</p>
            <button
              type="button"
              class="btn btn-secondary btn-sm mt-3 inline-flex items-center gap-2"
              data-testid="leaderboard-retry"
              @click="loadConfig"
            >
              <Icon name="refresh" size="sm" aria-hidden="true" />
              <span>{{ t('admin.leaderboard.retry') }}</span>
            </button>
          </div>
        </div>
      </div>

      <form v-else class="space-y-6" @submit.prevent="saveConfig">
        <section class="card p-6">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.leaderboard.switchTitle') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.leaderboard.switchHint') }}</p>
            </div>
            <Toggle v-model="form.enabled" data-testid="leaderboard-enabled" />
          </div>
          <p v-if="!form.enabled" class="mt-4 rounded-md border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">{{ t('admin.leaderboard.disabledHint') }}</p>
        </section>

        <section class="card space-y-5 p-6">
          <div class="flex items-center justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.leaderboard.weeklyTitle') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.leaderboard.rewardMetadataHint') }}</p>
            </div>
            <Toggle v-model="form.weekly_enabled" />
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <label class="space-y-1.5"><span class="input-label">{{ t('admin.leaderboard.topN') }}</span><input v-model.number="form.weekly_top_n" class="input" type="number" min="1" max="100" step="1" inputmode="numeric" /></label>
            <label class="space-y-1.5"><span class="input-label">{{ t('admin.leaderboard.defaultReward') }}</span><input v-model.number="form.weekly_reward" class="input" type="number" min="0" step="0.01" inputmode="decimal" /></label>
          </div>
          <label class="block space-y-1.5"><span class="input-label">{{ t('admin.leaderboard.rewards') }}</span><input v-model="form.weekly_rewards" class="input" type="text" :placeholder="t('admin.leaderboard.rewardsPlaceholder')" autocomplete="off" /><span class="input-hint">{{ t('admin.leaderboard.rewardsHint') }}</span></label>
        </section>

        <section class="card space-y-5 p-6">
          <div class="flex items-center justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.leaderboard.monthlyTitle') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.leaderboard.rewardMetadataHint') }}</p>
            </div>
            <Toggle v-model="form.monthly_enabled" />
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <label class="space-y-1.5"><span class="input-label">{{ t('admin.leaderboard.topN') }}</span><input v-model.number="form.monthly_top_n" class="input" type="number" min="1" max="100" step="1" inputmode="numeric" /></label>
            <label class="space-y-1.5"><span class="input-label">{{ t('admin.leaderboard.defaultReward') }}</span><input v-model.number="form.monthly_reward" class="input" type="number" min="0" step="0.01" inputmode="decimal" /></label>
          </div>
          <label class="block space-y-1.5"><span class="input-label">{{ t('admin.leaderboard.rewards') }}</span><input v-model="form.monthly_rewards" class="input" type="text" :placeholder="t('admin.leaderboard.rewardsPlaceholder')" autocomplete="off" /><span class="input-hint">{{ t('admin.leaderboard.rewardsHint') }}</span></label>
        </section>

        <div class="flex items-center justify-end gap-3">
          <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.leaderboard.readOnlySettlement') }}</span>
          <button type="submit" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" data-testid="leaderboard-save">
            <Icon name="refresh" size="sm" :class="saving ? 'animate-spin' : ''" aria-hidden="true" />
            <span>{{ t('common.save') }}</span>
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
import { getLeaderboardConfig, updateLeaderboardConfig, type LeaderboardConfig } from '@/api/leaderboard'
import { isStepUpBlocked, isStepUpCancelled, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const stepUp = useStepUp()
const loading = ref(true)
const loadFailed = ref(false)
const saving = ref(false)

const form = reactive({
  enabled: false,
  weekly_enabled: false,
  weekly_top_n: 3,
  weekly_reward: 1,
  weekly_rewards: '1, 1, 1',
  monthly_enabled: false,
  monthly_top_n: 3,
  monthly_reward: 5,
  monthly_rewards: '5, 5, 5',
})

function applyConfig(config: LeaderboardConfig): void {
  form.enabled = config?.enabled === true
  form.weekly_enabled = config?.weekly_enabled === true
  form.weekly_top_n = Number(config?.weekly_top_n) || 3
  form.weekly_reward = Number(config?.weekly_reward) || 0
  form.weekly_rewards = (config?.weekly_rewards ?? []).join(', ')
  form.monthly_enabled = config?.monthly_enabled === true
  form.monthly_top_n = Number(config?.monthly_top_n) || 3
  form.monthly_reward = Number(config?.monthly_reward) || 0
  form.monthly_rewards = (config?.monthly_rewards ?? []).join(', ')
  syncPublicFlag(form.enabled)
}

function syncPublicFlag(enabled: boolean): void {
  const current = appStore.cachedPublicSettings
  if (current) appStore.cachedPublicSettings = { ...current, subnexus_leaderboard_enabled: enabled }
  try { window.dispatchEvent(new CustomEvent('leaderboard-config-changed')) } catch { /* test/non-browser runtime */ }
}

function parseRewards(raw: string, field: string): number[] | null {
  if (!raw.trim()) return []
  const values = raw.split(',').map((part) => Number(part.trim()))
  if (values.some((value) => !Number.isFinite(value) || value < 0)) {
    appStore.showError(t('admin.leaderboard.invalidRewards', { field }))
    return null
  }
  return values.map((value) => Math.round(value * 100) / 100)
}

function buildConfig(): LeaderboardConfig | null {
  const weeklyRewards = parseRewards(form.weekly_rewards, t('admin.leaderboard.weeklyTitle'))
  const monthlyRewards = parseRewards(form.monthly_rewards, t('admin.leaderboard.monthlyTitle'))
  if (!weeklyRewards || !monthlyRewards) return null
  const weeklyTopN = Math.trunc(Number(form.weekly_top_n))
  const monthlyTopN = Math.trunc(Number(form.monthly_top_n))
  if (weeklyTopN < 1 || weeklyTopN > 100 || monthlyTopN < 1 || monthlyTopN > 100) {
    appStore.showError(t('admin.leaderboard.invalidTopN'))
    return null
  }
  const weeklyReward = Number(form.weekly_reward)
  const monthlyReward = Number(form.monthly_reward)
  if (!Number.isFinite(weeklyReward) || weeklyReward < 0 || !Number.isFinite(monthlyReward) || monthlyReward < 0) {
    appStore.showError(t('admin.leaderboard.invalidRewards', { field: t('admin.leaderboard.defaultReward') }))
    return null
  }
  return {
    enabled: form.enabled,
    weekly_enabled: form.weekly_enabled,
    weekly_top_n: weeklyTopN,
    weekly_reward: Math.round(weeklyReward * 100) / 100,
    weekly_rewards: weeklyRewards,
    monthly_enabled: form.monthly_enabled,
    monthly_top_n: monthlyTopN,
    monthly_reward: Math.round(monthlyReward * 100) / 100,
    monthly_rewards: monthlyRewards,
  }
}

async function loadConfig(): Promise<void> {
  loading.value = true
  loadFailed.value = false
  try { applyConfig(await getLeaderboardConfig()) }
  catch (error) {
    loadFailed.value = true
    syncPublicFlag(false)
    appStore.showError(extractApiErrorMessage(error, t('admin.leaderboard.loadFailed')))
  }
  finally { loading.value = false }
}

async function saveConfig(): Promise<void> {
  if (saving.value) return
  const config = buildConfig()
  if (!config) return
  saving.value = true
  try {
    applyConfig(await stepUp.run(() => updateLeaderboardConfig(config)))
    appStore.showSuccess(t('admin.leaderboard.saved'))
  } catch (error) {
    if (isStepUpCancelled(error)) return
    if (isStepUpBlocked(error)) {
      appStore.showError(stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled'))
      return
    }
    appStore.showError(extractApiErrorMessage(error, t('admin.leaderboard.saveFailed')))
  } finally { saving.value = false }
}

onMounted(() => { void loadConfig() })
</script>
