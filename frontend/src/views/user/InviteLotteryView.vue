<template>
  <AppLayout>
    <section class="mx-auto max-w-6xl space-y-6" data-testid="invite-lottery-page">
      <header class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div class="mb-2 flex items-center gap-2 text-amber-600 dark:text-amber-400">
            <Icon name="sparkles" size="md" aria-hidden="true" />
            <span class="text-xs font-semibold uppercase">{{ t('inviteActivities.inviteLottery.title') }}</span>
          </div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.inviteLottery.title') }}</h1>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-dark-400">{{ t('inviteActivities.inviteLottery.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading || claiming" @click="loadStatus">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
          <span>{{ t('inviteActivities.common.refresh') }}</span>
        </button>
      </header>

      <div v-if="loading" class="rounded-lg border border-gray-200 bg-white px-6 py-16 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400" aria-busy="true">
        {{ t('inviteActivities.common.loading') }}
      </div>

      <div v-else-if="disabled" class="rounded-lg border border-dashed border-gray-300 bg-white px-6 py-16 text-center dark:border-dark-600 dark:bg-dark-900" data-testid="invite-lottery-disabled">
        <Icon name="lock" size="xl" class="mx-auto text-gray-300 dark:text-dark-600" aria-hidden="true" />
        <p class="mt-4 text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('inviteActivities.common.disabled') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('inviteActivities.common.disabledHint') }}</p>
      </div>

      <template v-else-if="status">
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <article class="metric-block">
            <span>{{ t('inviteActivities.inviteLottery.remaining') }}</span>
            <strong>{{ status.remaining_chances }}</strong>
          </article>
          <article class="metric-block">
            <span>{{ t('inviteActivities.inviteLottery.invited') }}</span>
            <strong>{{ status.invited_count }}</strong>
          </article>
          <article v-if="status.recharge_limit_enabled" class="metric-block">
            <span>{{ t('inviteActivities.inviteLottery.qualified') }}</span>
            <strong>{{ status.qualified_invited_count }}</strong>
          </article>
          <article class="metric-block">
            <span>{{ t('inviteActivities.inviteLottery.used') }}</span>
            <strong>{{ status.used_chances }}</strong>
          </article>
        </div>

        <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
          <section class="lottery-board" :class="{ 'lottery-board--claiming': claiming }">
            <div class="mb-4 flex items-center justify-between gap-3">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('inviteActivities.inviteLottery.prizePool') }}</h2>
              <span class="text-xs tabular-nums text-gray-500 dark:text-dark-400">{{ status.prizes.length }} {{ t('inviteActivities.common.reward') }}</span>
            </div>
            <div class="grid auto-rows-fr grid-cols-2 gap-3 sm:grid-cols-3">
              <article
                v-for="(prize, index) in status.prizes"
                :key="`${prize.name}-${prize.amount}-${index}`"
                class="prize-tile"
                :class="{ 'prize-tile--won': lastPrize?.name === prize.name && lastPrize?.amount === prize.amount }"
              >
                <span class="prize-icon"><Icon name="gift" size="sm" aria-hidden="true" /></span>
                <span class="mt-3 min-w-0 break-words text-sm font-medium text-gray-800 dark:text-dark-100">{{ prize.name }}</span>
                <strong class="mt-1 text-base tabular-nums text-emerald-600 dark:text-emerald-400">${{ money(prize.amount) }}</strong>
              </article>
            </div>
          </section>

          <aside class="space-y-4">
            <div v-if="lastPrize" class="rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800 dark:border-emerald-900/60 dark:bg-emerald-950/30 dark:text-emerald-300" role="status" data-testid="invite-lottery-result">
              <div class="flex gap-3">
                <Icon name="checkCircle" size="md" class="mt-0.5 shrink-0" aria-hidden="true" />
                <p>{{ t('inviteActivities.inviteLottery.won', { name: lastPrize.name, amount: money(lastPrize.amount) }) }}</p>
              </div>
            </div>

            <div v-if="status.recharge_limit_enabled" class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/25 dark:text-amber-300">
              <div class="flex items-start gap-3">
                <Icon name="lock" size="sm" class="mt-0.5 shrink-0" aria-hidden="true" />
                <div>
                  <p>{{ t('inviteActivities.inviteLottery.lockedHint', { amount: money(status.invitee_recharge_threshold) }) }}</p>
                  <p v-if="status.locked_chances > 0" class="mt-2 font-semibold">{{ t('inviteActivities.inviteLottery.locked') }}: {{ status.locked_chances }}</p>
                </div>
              </div>
            </div>

            <button
              type="button"
              class="btn btn-primary inline-flex min-h-12 w-full items-center justify-center gap-2"
              :disabled="!status.can_claim || claiming"
              data-testid="invite-lottery-claim"
              @click="claim"
            >
              <Icon name="sparkles" size="sm" :class="claiming ? 'animate-pulse' : ''" aria-hidden="true" />
              <span>{{ claimLabel }}</span>
            </button>
            <button type="button" class="btn btn-secondary inline-flex w-full items-center justify-center gap-2" @click="goAffiliate">
              <Icon name="users" size="sm" aria-hidden="true" />
              <span>{{ t('inviteActivities.inviteLottery.inviteFriends') }}</span>
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
  claimInviteLottery,
  getInviteLotteryStatus,
  type InviteLotteryPrizePublic,
  type InviteLotteryStatus,
} from '@/api/inviteActivities'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isInviteActivitySettingsEnabled } from '@/utils/inviteActivities'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const status = ref<InviteLotteryStatus | null>(null)
const lastPrize = ref<InviteLotteryPrizePublic | null>(null)
const loading = ref(false)
const claiming = ref(false)
const featureDisabled = ref(false)

const disabled = computed(() => featureDisabled.value || status.value?.enabled === false)
const claimLabel = computed(() => {
  if (claiming.value) return t('inviteActivities.common.claiming')
  if (status.value?.can_claim) return t('inviteActivities.inviteLottery.draw')
  if (status.value?.locked_chances) return t('inviteActivities.inviteLottery.waitingRecharge')
  return t('inviteActivities.inviteLottery.noChance')
})

function money(value: number): string {
  return Number(value || 0).toFixed(2)
}

async function publicFlagEnabled(): Promise<boolean> {
  if (!appStore.publicSettingsLoaded) await appStore.fetchPublicSettings()
  return isInviteActivitySettingsEnabled(
    appStore.publicSettingsLoaded,
    appStore.cachedPublicSettings,
    'subnexus_invite_lottery_enabled',
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
    const result = await getInviteLotteryStatus()
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
  // The page can remain open while an administrator disables the activity.
  // Re-check the local rollout state before issuing a balance-affecting POST;
  // the backend repeats this check authoritatively as well.
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
  lastPrize.value = null
  try {
    const result = await claimInviteLottery()
    status.value = result
    if (result.prize) {
      lastPrize.value = result.prize
      appStore.showSuccess(t('inviteActivities.inviteLottery.won', {
        name: result.prize.name,
        amount: money(result.prize.amount),
      }))
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.claimFailed')))
    await loadStatus()
  } finally {
    claiming.value = false
  }
}

function goAffiliate(): void {
  void router.push('/affiliate')
}

onMounted(() => { void loadStatus() })
</script>

<style scoped>
.metric-block { @apply rounded-lg border border-gray-200 bg-white px-4 py-4 dark:border-dark-700 dark:bg-dark-900; }
.metric-block span { @apply block text-xs font-medium text-gray-500 dark:text-dark-400; }
.metric-block strong { @apply mt-2 block text-2xl font-semibold tabular-nums text-gray-900 dark:text-white; }
.lottery-board { @apply rounded-lg border border-gray-200 bg-gray-50 p-4 transition-opacity dark:border-dark-700 dark:bg-dark-900/60 sm:p-5; }
.lottery-board--claiming { @apply opacity-70; }
.prize-tile { @apply flex min-h-32 flex-col items-center justify-center rounded-lg border border-gray-200 bg-white p-3 text-center transition-colors dark:border-dark-700 dark:bg-dark-900; }
.prize-tile--won { @apply border-emerald-500 bg-emerald-50 ring-2 ring-emerald-500/20 dark:border-emerald-500 dark:bg-emerald-950/30; }
.prize-icon { @apply flex h-9 w-9 items-center justify-center rounded-full bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300; }
</style>
