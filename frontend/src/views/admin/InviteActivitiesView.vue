<template>
  <AppLayout>
    <section class="mx-auto max-w-5xl space-y-6" data-testid="admin-invite-activities-page">
      <header class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div class="flex items-center gap-2 text-primary-600 dark:text-primary-400">
            <Icon name="gift" size="md" aria-hidden="true" />
            <span class="text-xs font-semibold uppercase">{{ t('inviteActivities.admin.title') }}</span>
          </div>
          <h1 class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.admin.title') }}</h1>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-dark-400">{{ t('inviteActivities.admin.description') }}</p>
        </div>
        <button v-if="!loading && !loadFailed" type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" data-testid="invite-activities-save" @click="saveConfig">
          <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="saving ? 'animate-spin' : ''" aria-hidden="true" />
          <span>{{ saving ? t('inviteActivities.admin.saving') : t('inviteActivities.admin.save') }}</span>
        </button>
      </header>

      <div v-if="loading" class="flex items-center justify-center py-16" aria-busy="true">
        <Icon name="refresh" size="lg" class="animate-spin text-primary-500" aria-hidden="true" />
      </div>

      <div v-else-if="loadFailed" class="rounded-lg border border-red-200 bg-red-50 px-5 py-4 text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200" role="alert" data-testid="invite-activities-load-error">
        <div class="flex items-start gap-3">
          <Icon name="exclamationTriangle" size="md" class="mt-0.5 shrink-0" aria-hidden="true" />
          <div>
            <p class="text-sm font-medium">{{ t('inviteActivities.admin.loadFailed') }}</p>
            <button type="button" class="btn btn-secondary btn-sm mt-3 inline-flex items-center gap-2" @click="loadConfig">
              <Icon name="refresh" size="sm" aria-hidden="true" />
              <span>{{ t('inviteActivities.admin.retry') }}</span>
            </button>
          </div>
        </div>
      </div>

      <form v-else class="space-y-6" @submit.prevent="saveConfig">
        <section class="card p-6">
          <div class="flex items-start justify-between gap-4">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.admin.masterSwitch') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('inviteActivities.admin.masterHint') }}</p>
            </div>
            <Toggle v-model="form.enabled" data-testid="invite-activities-master-enabled" />
          </div>
          <div v-if="!form.enabled" class="mt-4 flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/25 dark:text-amber-300">
            <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0" aria-hidden="true" />
            <p>{{ t('inviteActivities.admin.rolloutWarning') }}</p>
          </div>
        </section>

        <div class="flex overflow-x-auto rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900" role="tablist">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            class="tab-button"
            :class="{ 'tab-button--active': activeTab === tab.key }"
            :aria-selected="activeTab === tab.key"
            role="tab"
            @click="activeTab = tab.key"
          >
            <Icon :name="tab.icon" size="sm" aria-hidden="true" />
            <span>{{ tab.label }}</span>
            <span class="h-2 w-2 rounded-full" :class="form[tab.switchKey] ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-dark-600'" aria-hidden="true"></span>
          </button>
        </div>

        <section v-show="activeTab === 'lottery'" class="card space-y-6 p-6" role="tabpanel">
          <div class="flex items-start justify-between gap-4">
            <div><h2 class="panel-title">{{ t('inviteActivities.admin.lotteryTab') }}</h2><p class="panel-hint">{{ t('inviteActivities.admin.lotteryHint') }}</p></div>
            <Toggle v-model="form.invite_lottery_enabled" data-testid="invite-lottery-enabled" />
          </div>
          <div class="grid gap-4 md:grid-cols-2">
            <label class="toggle-field md:col-span-2">
              <span><strong>{{ t('inviteActivities.admin.rechargeLimit') }}</strong></span>
              <Toggle v-model="form.invite_lottery_recharge_limit_enabled" />
            </label>
            <label v-if="form.invite_lottery_recharge_limit_enabled" class="field">
              <span class="input-label">{{ t('inviteActivities.admin.inviteeThreshold') }}</span>
              <input v-model.number="form.invite_lottery_recharge_threshold" class="input" type="number" min="0.01" step="0.01" inputmode="decimal" required />
            </label>
          </div>
          <div class="space-y-3">
            <div v-for="(prize, index) in form.invite_lottery_prizes" :key="`lottery-${index}`" class="config-row config-row--prize">
              <label class="field"><span class="input-label">{{ t('inviteActivities.admin.prizeName') }}</span><input v-model.trim="prize.name" class="input" type="text" maxlength="80" required /></label>
              <label class="field"><span class="input-label">{{ t('inviteActivities.admin.amount') }}</span><input v-model.number="prize.amount" class="input" type="number" min="0.01" step="0.01" required /></label>
              <label class="field"><span class="input-label">{{ t('inviteActivities.admin.probability') }}</span><input v-model.number="prize.probability" class="input" type="number" min="0" step="0.01" required /></label>
              <button type="button" class="icon-button" :title="t('inviteActivities.admin.remove')" :disabled="form.invite_lottery_prizes.length <= 1" @click="removeAt(form.invite_lottery_prizes, index)"><Icon name="trash" size="sm" aria-hidden="true" /></button>
            </div>
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="form.invite_lottery_prizes.length >= 50" @click="addLotteryPrize"><Icon name="plus" size="sm" aria-hidden="true" /><span>{{ t('inviteActivities.admin.addPrize') }}</span></button>
          </div>
        </section>

        <section v-show="activeTab === 'wheel'" class="card space-y-6 p-6" role="tabpanel">
          <div class="flex items-start justify-between gap-4">
            <div><h2 class="panel-title">{{ t('inviteActivities.admin.wheelTab') }}</h2><p class="panel-hint">{{ t('inviteActivities.admin.wheelHint') }}</p></div>
            <Toggle v-model="form.recharge_wheel_enabled" data-testid="recharge-wheel-enabled" />
          </div>
          <label class="field max-w-sm"><span class="input-label">{{ t('inviteActivities.admin.wheelThreshold') }}</span><input v-model.number="form.recharge_wheel_threshold" class="input" type="number" min="0.01" step="0.01" required /></label>
          <div class="grid gap-6 lg:grid-cols-2">
            <div class="space-y-3">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.rechargeWheel.amountPool') }}</h3>
              <div v-for="(item, index) in form.recharge_wheel_amounts" :key="`amount-${index}`" class="config-row config-row--compact">
                <label class="field"><span class="input-label">{{ t('inviteActivities.admin.amount') }}</span><input v-model.number="item.amount" class="input" type="number" min="0.01" step="0.01" required /></label>
                <label class="field"><span class="input-label">{{ t('inviteActivities.admin.probability') }}</span><input v-model.number="item.probability" class="input" type="number" min="0" step="0.01" required /></label>
                <button type="button" class="icon-button" :title="t('inviteActivities.admin.remove')" :disabled="form.recharge_wheel_amounts.length <= 1" @click="removeAt(form.recharge_wheel_amounts, index)"><Icon name="trash" size="sm" aria-hidden="true" /></button>
              </div>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="form.recharge_wheel_amounts.length >= 50" @click="addWheelAmount"><Icon name="plus" size="sm" aria-hidden="true" /><span>{{ t('inviteActivities.admin.addAmount') }}</span></button>
            </div>
            <div class="space-y-3">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.rechargeWheel.multiplierPool') }}</h3>
              <div v-for="(item, index) in form.recharge_wheel_multipliers" :key="`multiplier-${index}`" class="config-row config-row--compact">
                <label class="field"><span class="input-label">{{ t('inviteActivities.admin.multiplier') }}</span><input v-model.number="item.multiplier" class="input" type="number" min="0.01" step="0.01" required /></label>
                <label class="field"><span class="input-label">{{ t('inviteActivities.admin.probability') }}</span><input v-model.number="item.probability" class="input" type="number" min="0" step="0.01" required /></label>
                <button type="button" class="icon-button" :title="t('inviteActivities.admin.remove')" :disabled="form.recharge_wheel_multipliers.length <= 1" @click="removeAt(form.recharge_wheel_multipliers, index)"><Icon name="trash" size="sm" aria-hidden="true" /></button>
              </div>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="form.recharge_wheel_multipliers.length >= 50" @click="addWheelMultiplier"><Icon name="plus" size="sm" aria-hidden="true" /><span>{{ t('inviteActivities.admin.addMultiplier') }}</span></button>
            </div>
          </div>
        </section>

        <section v-show="activeTab === 'milestone'" class="card space-y-6 p-6" role="tabpanel">
          <div class="flex items-start justify-between gap-4">
            <div><h2 class="panel-title">{{ t('inviteActivities.admin.milestoneTab') }}</h2><p class="panel-hint">{{ t('inviteActivities.admin.milestoneHint') }}</p></div>
            <Toggle v-model="form.invite_milestone_enabled" data-testid="invite-milestone-enabled" />
          </div>
          <div class="grid gap-4 md:grid-cols-2">
            <label class="toggle-field md:col-span-2"><span><strong>{{ t('inviteActivities.admin.rechargeLimit') }}</strong></span><Toggle v-model="form.invite_milestone_recharge_limit_enabled" /></label>
            <label v-if="form.invite_milestone_recharge_limit_enabled" class="field"><span class="input-label">{{ t('inviteActivities.admin.inviteeThreshold') }}</span><input v-model.number="form.invite_milestone_recharge_threshold" class="input" type="number" min="0.01" step="0.01" required /></label>
          </div>
          <div class="space-y-3">
            <div v-for="(tier, index) in form.invite_milestone_tiers" :key="`tier-${index}`" class="config-row config-row--compact">
              <label class="field"><span class="input-label">{{ t('inviteActivities.admin.invites') }}</span><input v-model.number="tier.invites" class="input" type="number" min="1" step="1" required /></label>
              <label class="field"><span class="input-label">{{ t('inviteActivities.admin.amount') }}</span><input v-model.number="tier.reward" class="input" type="number" min="0.01" step="0.01" required /></label>
              <button type="button" class="icon-button" :title="t('inviteActivities.admin.remove')" :disabled="form.invite_milestone_tiers.length <= 1" @click="removeAt(form.invite_milestone_tiers, index)"><Icon name="trash" size="sm" aria-hidden="true" /></button>
            </div>
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="form.invite_milestone_tiers.length >= 20" @click="addMilestoneTier"><Icon name="plus" size="sm" aria-hidden="true" /><span>{{ t('inviteActivities.admin.addTier') }}</span></button>
          </div>
        </section>

        <div class="flex justify-end">
          <button type="submit" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" data-testid="invite-activities-save-bottom">
            <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="saving ? 'animate-spin' : ''" aria-hidden="true" />
            <span>{{ saving ? t('inviteActivities.admin.saving') : t('inviteActivities.admin.save') }}</span>
          </button>
        </div>
      </form>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import { getInviteActivitiesConfig, updateInviteActivitiesConfig, type InviteActivitiesConfig } from '@/api/inviteActivities'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

type ActivityTab = 'lottery' | 'wheel' | 'milestone'
type ConfigArray = InviteActivitiesConfig['invite_lottery_prizes'] | InviteActivitiesConfig['recharge_wheel_amounts'] | InviteActivitiesConfig['recharge_wheel_multipliers'] | InviteActivitiesConfig['invite_milestone_tiers']

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const loadFailed = ref(false)
const saving = ref(false)
const activeTab = ref<ActivityTab>('lottery')
const form = reactive<InviteActivitiesConfig>(defaultConfig())
const tabs = [
  { key: 'lottery' as const, label: t('inviteActivities.admin.lotteryTab'), icon: 'users' as const, switchKey: 'invite_lottery_enabled' as const },
  { key: 'wheel' as const, label: t('inviteActivities.admin.wheelTab'), icon: 'refresh' as const, switchKey: 'recharge_wheel_enabled' as const },
  { key: 'milestone' as const, label: t('inviteActivities.admin.milestoneTab'), icon: 'chartBar' as const, switchKey: 'invite_milestone_enabled' as const },
]

function defaultConfig(): InviteActivitiesConfig {
  return {
    enabled: false,
    invite_lottery_enabled: false,
    invite_lottery_prizes: [
      { name: 'Lucky reward', amount: 0.1, probability: 50 },
      { name: 'Advanced reward', amount: 0.5, probability: 30 },
      { name: 'Super reward', amount: 1, probability: 20 },
    ],
    invite_lottery_recharge_limit_enabled: false,
    invite_lottery_recharge_threshold: 10,
    recharge_wheel_enabled: false,
    recharge_wheel_threshold: 10,
    recharge_wheel_amounts: [
      { amount: 0.5, probability: 40 }, { amount: 1, probability: 30 },
      { amount: 2, probability: 20 }, { amount: 5, probability: 10 },
    ],
    recharge_wheel_multipliers: [
      { multiplier: 1, probability: 40 }, { multiplier: 2, probability: 30 },
      { multiplier: 3, probability: 20 }, { multiplier: 5, probability: 10 },
    ],
    invite_milestone_enabled: false,
    invite_milestone_tiers: [
      { invites: 5, reward: 1 }, { invites: 10, reward: 3 },
      { invites: 20, reward: 8 }, { invites: 50, reward: 25 },
    ],
    invite_milestone_recharge_limit_enabled: false,
    invite_milestone_recharge_threshold: 10,
  }
}

function cloneConfig(config: InviteActivitiesConfig): InviteActivitiesConfig {
  return {
    ...config,
    invite_lottery_prizes: config.invite_lottery_prizes.map((item) => ({ ...item })),
    recharge_wheel_amounts: config.recharge_wheel_amounts.map((item) => ({ ...item })),
    recharge_wheel_multipliers: config.recharge_wheel_multipliers.map((item) => ({ ...item })),
    invite_milestone_tiers: config.invite_milestone_tiers.map((item) => ({ ...item })),
  }
}

function applyConfig(config: InviteActivitiesConfig): void { Object.assign(form, cloneConfig(config)) }
function removeAt(list: ConfigArray, index: number): void { if (list.length > 1) list.splice(index, 1) }
function addLotteryPrize(): void { if (form.invite_lottery_prizes.length < 50) form.invite_lottery_prizes.push({ name: `Reward ${form.invite_lottery_prizes.length + 1}`, amount: 0.1, probability: 10 }) }
function addWheelAmount(): void { if (form.recharge_wheel_amounts.length < 50) form.recharge_wheel_amounts.push({ amount: 1, probability: 10 }) }
function addWheelMultiplier(): void { if (form.recharge_wheel_multipliers.length < 50) form.recharge_wheel_multipliers.push({ multiplier: 1, probability: 10 }) }
function addMilestoneTier(): void {
  if (form.invite_milestone_tiers.length >= 20) return
  const highest = Math.max(0, ...form.invite_milestone_tiers.map((tier) => Number(tier.invites) || 0))
  form.invite_milestone_tiers.push({ invites: highest > 0 ? highest * 2 : 5, reward: 1 })
}

function positive(value: number): boolean { return Number.isFinite(Number(value)) && Number(value) > 0 }
function nonNegative(value: number): boolean { return Number.isFinite(Number(value)) && Number(value) >= 0 }

function validConfig(config: InviteActivitiesConfig): boolean {
  if (config.invite_lottery_prizes.length < 1 || config.recharge_wheel_amounts.length < 1 || config.recharge_wheel_multipliers.length < 1 || config.invite_milestone_tiers.length < 1) {
    appStore.showError(t('inviteActivities.admin.emptyList'))
    return false
  }
  if (config.invite_lottery_prizes.length > 50 || config.recharge_wheel_amounts.length > 50 || config.recharge_wheel_multipliers.length > 50 || config.invite_milestone_tiers.length > 20) {
    appStore.showError(t('inviteActivities.admin.itemLimit'))
    return false
  }
  const lotteryValid = config.invite_lottery_prizes.every((item) => item.name.trim() !== '' && positive(item.amount) && nonNegative(item.probability))
  const amountsValid = config.recharge_wheel_amounts.every((item) => positive(item.amount) && nonNegative(item.probability))
  const multipliersValid = config.recharge_wheel_multipliers.every((item) => positive(item.multiplier) && nonNegative(item.probability))
  const tiersValid = config.invite_milestone_tiers.every((item) => Number.isInteger(Number(item.invites)) && positive(item.invites) && positive(item.reward))
  const inviteTargets = config.invite_milestone_tiers.map((tier) => Number(tier.invites))
  if (new Set(inviteTargets).size !== inviteTargets.length) {
    appStore.showError(t('inviteActivities.admin.duplicateTier'))
    return false
  }
  const enabledPoolsValid =
    (!config.invite_lottery_enabled || config.invite_lottery_prizes.some((item) => item.probability > 0)) &&
    (!config.recharge_wheel_enabled || (config.recharge_wheel_amounts.some((item) => item.probability > 0) && config.recharge_wheel_multipliers.some((item) => item.probability > 0)))
  const thresholdsValid =
    (!config.invite_lottery_recharge_limit_enabled || positive(config.invite_lottery_recharge_threshold)) &&
    (!config.recharge_wheel_enabled || positive(config.recharge_wheel_threshold)) &&
    (!config.invite_milestone_recharge_limit_enabled || positive(config.invite_milestone_recharge_threshold))
  if (!lotteryValid || !amountsValid || !multipliersValid || !tiersValid || !enabledPoolsValid || !thresholdsValid) {
    appStore.showError(t('inviteActivities.admin.invalidConfig'))
    return false
  }
  return true
}

function syncPublicFlags(config: InviteActivitiesConfig): void {
  if (!appStore.cachedPublicSettings) return
  appStore.cachedPublicSettings = {
    ...appStore.cachedPublicSettings,
    subnexus_invite_activities_enabled: config.enabled,
    subnexus_invite_lottery_enabled: config.enabled && config.invite_lottery_enabled,
    subnexus_recharge_wheel_enabled: config.enabled && config.recharge_wheel_enabled,
    subnexus_invite_milestone_enabled: config.enabled && config.invite_milestone_enabled,
  }
}

async function loadConfig(): Promise<void> {
  loading.value = true
  loadFailed.value = false
  try { applyConfig(await getInviteActivitiesConfig()) }
  catch (error) {
    loadFailed.value = true
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.admin.loadFailed')))
  } finally { loading.value = false }
}

async function saveConfig(): Promise<void> {
  if (saving.value) return
  const config = cloneConfig(form)
  if (!validConfig(config)) return
  saving.value = true
  try {
    const saved = await updateInviteActivitiesConfig(config)
    applyConfig(saved)
    syncPublicFlags(saved)
    appStore.showSuccess(t('inviteActivities.admin.saved'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.admin.saveFailed')))
  } finally { saving.value = false }
}

onMounted(() => { void loadConfig() })
</script>

<style scoped>
.tab-button { @apply inline-flex min-w-44 flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium text-gray-600 transition-colors hover:text-gray-900 dark:text-dark-300 dark:hover:text-white; }
.tab-button--active { @apply bg-primary-600 text-white shadow-sm hover:text-white; }
.panel-title { @apply text-base font-semibold text-gray-900 dark:text-white; }
.panel-hint { @apply mt-1 text-sm text-gray-500 dark:text-dark-400; }
.field { @apply block min-w-0 space-y-1.5; }
.input-label { @apply block text-xs font-medium text-gray-600 dark:text-dark-300; }
.toggle-field { @apply flex items-center justify-between gap-4 rounded-lg border border-gray-200 px-4 py-3 text-sm text-gray-700 dark:border-dark-700 dark:text-dark-200; }
.config-row { @apply grid items-end gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800/40; }
.config-row--prize { grid-template-columns:minmax(160px,2fr) minmax(100px,1fr) minmax(110px,1fr) 40px; }
.config-row--compact { grid-template-columns:minmax(100px,1fr) minmax(110px,1fr) 40px; }
.icon-button { @apply inline-flex h-10 w-10 items-center justify-center rounded-md border border-gray-200 bg-white text-gray-500 transition-colors hover:border-red-300 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:bg-dark-900 dark:text-dark-300 dark:hover:border-red-800 dark:hover:text-red-400; }
@media (max-width: 640px) {
  .config-row--prize,.config-row--compact { grid-template-columns:minmax(0,1fr) minmax(0,1fr); }
  .config-row--prize .field:first-child { grid-column:1 / -1; }
  .icon-button { grid-column:2; justify-self:end; }
}
</style>
