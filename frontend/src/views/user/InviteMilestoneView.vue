<template>
  <AppLayout>
    <section class="mx-auto max-w-6xl space-y-6" data-testid="invite-milestone-page">
      <header class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div class="mb-2 flex items-center gap-2 text-emerald-700 dark:text-emerald-400">
            <Icon name="chartBar" size="md" aria-hidden="true" />
            <span class="text-xs font-semibold uppercase">{{ t('inviteActivities.inviteMilestone.title') }}</span>
          </div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.inviteMilestone.title') }}</h1>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-dark-400">{{ t('inviteActivities.inviteMilestone.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading || claimingTarget !== null" @click="loadStatus">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
          <span>{{ t('inviteActivities.common.refresh') }}</span>
        </button>
      </header>

      <div v-if="loading" class="rounded-lg border border-gray-200 bg-white px-6 py-16 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400" aria-busy="true">
        {{ t('inviteActivities.common.loading') }}
      </div>

      <div v-else-if="disabled" class="rounded-lg border border-dashed border-gray-300 bg-white px-6 py-16 text-center dark:border-dark-600 dark:bg-dark-900" data-testid="invite-milestone-disabled">
        <Icon name="lock" size="xl" class="mx-auto text-gray-300 dark:text-dark-600" aria-hidden="true" />
        <p class="mt-4 text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('inviteActivities.common.disabled') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('inviteActivities.common.disabledHint') }}</p>
      </div>

      <template v-else-if="status">
        <div class="grid gap-3 sm:grid-cols-2">
          <article class="metric-block"><span>{{ t('inviteActivities.inviteMilestone.invited') }}</span><strong>{{ status.invited_count }}</strong></article>
          <article v-if="status.recharge_limit_enabled" class="metric-block"><span>{{ t('inviteActivities.inviteMilestone.qualified') }}</span><strong>{{ status.qualified_invited_count }}</strong></article>
        </div>

        <div v-if="status.recharge_limit_enabled" class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/25 dark:text-amber-300">
          {{ t('inviteActivities.inviteMilestone.qualificationHint', { amount: money(status.invitee_recharge_threshold) }) }}
        </div>

        <section class="space-y-4" aria-labelledby="milestone-progress-title">
          <div class="flex items-center justify-between gap-4">
            <h2 id="milestone-progress-title" class="text-base font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.inviteMilestone.progress') }}</h2>
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="goAffiliate">
              <Icon name="users" size="sm" aria-hidden="true" />
              <span>{{ t('inviteActivities.inviteMilestone.inviteFriends') }}</span>
            </button>
          </div>

          <div class="h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700" aria-hidden="true">
            <div class="h-full rounded-full bg-emerald-500 transition-[width] duration-500" :style="{ width: `${progressPercent}%` }"></div>
          </div>

          <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <article
              v-for="tier in status.tiers"
              :key="tier.invites"
              class="tier-card"
              :class="{
                'tier-card--claimable': tier.claimable,
                'tier-card--claimed': tier.claimed,
                'tier-card--locked': !tier.reached || tier.blocked_by_recharge,
              }"
            >
              <div class="flex items-start justify-between gap-4">
                <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-300">
                  <Icon :name="tier.claimed ? 'checkCircle' : tier.blocked_by_recharge ? 'lock' : 'gift'" size="md" aria-hidden="true" />
                </span>
                <strong class="text-xl tabular-nums text-emerald-600 dark:text-emerald-400">${{ money(tier.reward) }}</strong>
              </div>
              <h3 class="mt-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.inviteMilestone.tierTarget', { count: tier.invites }) }}</h3>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ tierLabel(tier) }}</p>
              <button
                type="button"
                class="btn mt-4 w-full"
                :class="tier.claimable ? 'btn-primary' : 'btn-secondary'"
                :disabled="!tier.claimable || claimingTarget !== null"
                :data-testid="`invite-milestone-claim-${tier.invites}`"
                @click="claim(tier)"
              >
                {{ claimingTarget === tier.invites ? t('inviteActivities.common.claiming') : tierAction(tier) }}
              </button>
            </article>
          </div>
        </section>
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
  claimInviteMilestone,
  getInviteMilestoneStatus,
  type InviteMilestoneStatus,
  type InviteMilestoneTierStatus,
} from '@/api/inviteActivities'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isInviteActivitySettingsEnabled } from '@/utils/inviteActivities'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const status = ref<InviteMilestoneStatus | null>(null)
const loading = ref(false)
const claimingTarget = ref<number | null>(null)
const featureDisabled = ref(false)
const disabled = computed(() => featureDisabled.value || status.value?.enabled === false)
const progressPercent = computed(() => {
  const lastTarget = status.value?.tiers.at(-1)?.invites ?? 0
  if (lastTarget <= 0) return 0
  return Math.min(100, Math.max(0, (Number(status.value?.invited_count || 0) / lastTarget) * 100))
})

function money(value: number): string { return Number(value || 0).toFixed(2) }
function tierLabel(tier: InviteMilestoneTierStatus): string {
  if (tier.claimed) return t('inviteActivities.common.claimed')
  if (tier.blocked_by_recharge) return t('inviteActivities.inviteMilestone.waitingRecharge')
  if (!tier.reached) return t('inviteActivities.inviteMilestone.notReached')
  return t('inviteActivities.inviteMilestone.claim', { amount: money(tier.reward) })
}
function tierAction(tier: InviteMilestoneTierStatus): string {
  if (tier.claimed) return t('inviteActivities.inviteMilestone.claimedReward', { amount: money(tier.reward) })
  if (tier.blocked_by_recharge) return t('inviteActivities.inviteMilestone.waitingRecharge')
  if (!tier.reached) return t('inviteActivities.inviteMilestone.notReached')
  return t('inviteActivities.inviteMilestone.claim', { amount: money(tier.reward) })
}

async function publicFlagEnabled(): Promise<boolean> {
  if (!appStore.publicSettingsLoaded) await appStore.fetchPublicSettings()
  return isInviteActivitySettingsEnabled(
    appStore.publicSettingsLoaded,
    appStore.cachedPublicSettings,
    'subnexus_invite_milestone_enabled',
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
    const result = await getInviteMilestoneStatus()
    status.value = result
    featureDisabled.value = !result.enabled
  } catch (error) {
    status.value = null
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function claim(tier: InviteMilestoneTierStatus): Promise<void> {
  if (!tier.claimable || claimingTarget.value !== null) return
  claimingTarget.value = tier.invites
  // Avoid issuing a reward POST from a stale page after the activity has been
  // switched off. The service performs the authoritative repeat check.
  let enabled = false
  try {
    enabled = await publicFlagEnabled()
  } catch {
    enabled = false
  }
  if (!enabled) {
    featureDisabled.value = true
    status.value = null
    claimingTarget.value = null
    return
  }
  try {
    const result = await claimInviteMilestone(tier.invites)
    status.value = result
    const reward = result.just_claimed_reward ?? tier.reward
    appStore.showSuccess(t('inviteActivities.inviteMilestone.received', { amount: money(reward) }))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.claimFailed')))
    await loadStatus()
  } finally {
    claimingTarget.value = null
  }
}

function goAffiliate(): void { void router.push('/affiliate') }
onMounted(() => { void loadStatus() })
</script>

<style scoped>
.metric-block { @apply rounded-lg border border-gray-200 bg-white px-4 py-4 dark:border-dark-700 dark:bg-dark-900; }
.metric-block span { @apply block text-xs font-medium text-gray-500 dark:text-dark-400; }
.metric-block strong { @apply mt-2 block text-2xl font-semibold tabular-nums text-gray-900 dark:text-white; }
.tier-card { @apply rounded-lg border border-gray-200 bg-white p-5 transition-colors dark:border-dark-700 dark:bg-dark-900; }
.tier-card--claimable { @apply border-emerald-400 bg-emerald-50/50 dark:border-emerald-700 dark:bg-emerald-950/20; }
.tier-card--claimed { @apply border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800/60; }
.tier-card--locked { @apply opacity-75; }
</style>
