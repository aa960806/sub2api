<template>
  <AppLayout>
    <section class="space-y-6" data-testid="leaderboard-page">
      <header class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <div class="mb-2 flex items-center gap-2 text-primary-600 dark:text-primary-400">
            <Icon name="trophy" size="md" aria-hidden="true" />
            <span class="text-xs font-semibold uppercase">{{ t('nav.leaderboard') }}</span>
          </div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('leaderboard.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.description') }}</p>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <div class="inline-flex rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900" role="tablist">
            <button
              v-for="item in boardTypeOptions"
              :key="item.value"
              type="button"
              class="seg-btn"
              :class="{ active: boardType === item.value }"
              :aria-selected="boardType === item.value"
              role="tab"
              @click="switchBoardType(item.value)"
            >{{ item.label }}</button>
          </div>
          <div class="inline-flex rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900" role="tablist">
            <button
              v-for="item in windowOptions"
              :key="item.value"
              type="button"
              class="seg-btn"
              :class="{ active: activeWindow === item.value }"
              :aria-selected="activeWindow === item.value"
              role="tab"
              @click="switchWindow(item.value)"
            >{{ item.label }}</button>
          </div>
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center gap-2"
            :disabled="loading"
            :title="t('leaderboard.refresh')"
            @click="loadData"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
            <span>{{ t('leaderboard.refresh') }}</span>
          </button>
        </div>
      </header>

      <div v-if="loading" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" aria-busy="true">
        <div v-for="item in 3" :key="item" class="h-28 animate-pulse rounded-lg border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-800"></div>
      </div>

      <div v-else-if="leaderboardDisabled" class="rounded-lg border border-dashed border-gray-300 bg-white px-6 py-14 text-center dark:border-dark-600 dark:bg-dark-800" data-testid="leaderboard-disabled">
        <Icon name="trophy" size="xl" class="mx-auto text-gray-300 dark:text-dark-600" aria-hidden="true" />
        <p class="mt-4 text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('leaderboard.disabled') }}</p>
      </div>

      <template v-else>
        <div v-if="boardType === 'usage'" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.totalUsage') }}</p>
            <p class="mt-3 text-2xl font-semibold text-gray-900 dark:text-white">${{ formatMoney(board?.total_usage) }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ periodText(board) }}</p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.totalRequests') }}</p>
            <p class="mt-3 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatCount(board?.requests) }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ periodText(board) }}</p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.totalTokens') }}</p>
            <p class="mt-3 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatToken(board?.tokens) }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('leaderboard.tokenHint') }}</p>
          </div>
        </div>
        <div v-else class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.totalInvites') }}</p>
          <p class="mt-3 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatCount(inviteBoard?.total_invites) }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ periodText(inviteBoard) }}</p>
        </div>

        <div v-if="boardType === 'usage' && myEntry" class="card flex items-center gap-4 border-l-4 border-primary-500 p-5" data-testid="leaderboard-my-rank">
          <span class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300">
            <Icon name="chartBar" size="md" aria-hidden="true" />
          </span>
          <div class="min-w-0">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.myRank') }}</p>
            <p class="mt-0.5 text-lg font-semibold text-gray-900 dark:text-white">#{{ myEntry.rank }} <span class="text-sm font-normal text-gray-500 dark:text-dark-400">${{ formatMoney(myEntry.usage) }} · {{ formatCount(myEntry.requests) }} {{ t('leaderboard.requests').toLowerCase() }}</span></p>
          </div>
        </div>

        <div class="card overflow-hidden">
          <div class="flex flex-col gap-2 border-b border-gray-200 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ boardType === 'usage' ? t('leaderboard.topUsers') : t('leaderboard.topInviters') }}</h2>
              <p class="text-sm text-gray-500 dark:text-dark-400">{{ periodText(boardType === 'usage' ? board : inviteBoard) }}</p>
            </div>
            <p v-if="rewardHint" class="text-sm font-medium text-amber-600 dark:text-amber-300">{{ rewardHint }}</p>
          </div>

          <div class="overflow-x-auto">
            <table v-if="boardType === 'usage'" class="w-full min-w-[760px] divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/80"><tr><th class="table-th text-center">{{ t('leaderboard.rank') }}</th><th class="table-th">{{ t('leaderboard.user') }}</th><th class="table-th numeric-cell">{{ t('leaderboard.usageAmount') }}</th><th class="table-th numeric-cell">{{ t('leaderboard.requests') }}</th><th class="table-th numeric-cell">{{ t('leaderboard.tokens') }}</th><th class="table-th numeric-cell">{{ t('leaderboard.reward') }}</th></tr></thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr v-if="!entries.length"><td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.empty') }}</td></tr>
                <tr v-for="entry in entries" :key="entry.user_id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60" :class="{ 'me-row': isMe(entry) }">
                  <td class="table-td text-center"><span class="rank-badge" :class="rankClass(entry.rank)">{{ entry.rank }}</span></td>
                  <td class="table-td"><span class="flex items-center gap-2"><span class="block max-w-[18rem] truncate font-medium text-gray-900 dark:text-white" :title="entry.email">{{ entry.email }}</span><span v-if="isMe(entry)" class="badge badge-primary shrink-0">{{ t('leaderboard.me') }}</span></span></td>
                  <td class="table-td numeric-cell font-semibold text-gray-900 dark:text-white">${{ formatMoney(entry.usage) }}</td>
                  <td class="table-td numeric-cell">{{ formatCount(entry.requests) }}</td>
                  <td class="table-td numeric-cell">{{ formatToken(entry.tokens) }}</td>
                  <td class="table-td numeric-cell"><span v-if="entry.reward_amount" class="badge badge-warning">${{ formatMoney(entry.reward_amount) }}<span v-if="entry.rewarded"> {{ t('leaderboard.rewarded') }}</span></span><span v-else class="text-gray-400">-</span></td>
                </tr>
              </tbody>
            </table>
            <table v-else class="w-full min-w-[520px] divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/80"><tr><th class="table-th text-center">{{ t('leaderboard.rank') }}</th><th class="table-th">{{ t('leaderboard.user') }}</th><th class="table-th numeric-cell">{{ t('leaderboard.inviteCount') }}</th></tr></thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr v-if="!inviteEntries.length"><td colspan="3" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.empty') }}</td></tr>
                <tr v-for="entry in inviteEntries" :key="entry.user_id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60" :class="{ 'me-row': isMe(entry) }">
                  <td class="table-td text-center"><span class="rank-badge" :class="rankClass(entry.rank)">{{ entry.rank }}</span></td>
                  <td class="table-td"><span class="flex items-center gap-2"><span class="block max-w-[24rem] truncate font-medium text-gray-900 dark:text-white" :title="entry.email">{{ entry.email }}</span><span v-if="isMe(entry)" class="badge badge-primary shrink-0">{{ t('leaderboard.me') }}</span></span></td>
                  <td class="table-td numeric-cell font-semibold text-gray-900 dark:text-white">{{ formatCount(entry.invite_count) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { getInviteLeaderboard, getLeaderboard, type InviteLeaderboardResponse, type LeaderboardResponse, type LeaderboardWindow } from '@/api/leaderboard'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorCode, extractApiErrorMessage } from '@/utils/apiError'
import { isLeaderboardSettingsEnabled } from '@/utils/leaderboard'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const activeWindow = ref<LeaderboardWindow>('week')
const boardType = ref<'usage' | 'invite'>('usage')
const board = ref<LeaderboardResponse | null>(null)
const inviteBoard = ref<InviteLeaderboardResponse | null>(null)
const loading = ref(false)
const leaderboardDisabled = ref(false)

const windowOptions = [
  { value: 'today' as const, label: t('leaderboard.today') },
  { value: 'week' as const, label: t('leaderboard.week') },
  { value: 'month' as const, label: t('leaderboard.month') },
]
const boardTypeOptions = [
  { value: 'usage' as const, label: t('leaderboard.usage') },
  { value: 'invite' as const, label: t('leaderboard.invites') },
]
const entries = computed(() => board.value?.entries ?? [])
const inviteEntries = computed(() => inviteBoard.value?.entries ?? [])
const currentUserId = computed(() => authStore.user?.id ?? null)
const myEntry = computed(() => entries.value.find((entry) => entry.user_id === currentUserId.value) ?? null)
const rewardHint = computed(() => {
  if (!board.value?.reward_amounts?.length) return ''
  const rewards = board.value.reward_amounts
    .map((amount, index) => amount > 0 ? t('leaderboard.rewardItem', { rank: index + 1, amount: formatMoney(amount) }) : '')
    .filter(Boolean)
    .join(' / ')
  return rewards ? t('leaderboard.rewardHint', { period: activeWindow.value === 'week' ? t('leaderboard.week') : t('leaderboard.month'), rewards }) : ''
})

function periodText(value: { start_date?: string; end_date?: string } | null | undefined): string {
  return value?.start_date && value?.end_date ? t('leaderboard.period', { start: value.start_date, end: value.end_date }) : ''
}
function formatMoney(value: number | undefined): string { return Number(value ?? 0).toFixed(2) }
function formatCount(value: number | undefined): string { return Number(value ?? 0).toLocaleString() }
function formatToken(value: number | undefined): string {
  const number = Number(value ?? 0)
  if (number >= 100000000) return `${(number / 100000000).toFixed(1)}亿`
  if (number >= 10000) return `${(number / 10000).toFixed(1)}万`
  return number.toLocaleString()
}
function rankClass(rank: number): string {
  return rank === 1 ? 'rank-gold' : rank === 2 ? 'rank-silver' : rank === 3 ? 'rank-bronze' : ''
}
function isMe(entry: { user_id: number }): boolean { return currentUserId.value != null && entry.user_id === currentUserId.value }

async function loadData(): Promise<void> {
  loading.value = true
  try {
    // Direct component mounts (deep links, stale router state) must obey the
    // same fail-closed flag as the route and must not issue board queries while off.
    if (!appStore.publicSettingsLoaded) await appStore.fetchPublicSettings()
    if (!isLeaderboardSettingsEnabled(appStore.publicSettingsLoaded, appStore.cachedPublicSettings)) {
      leaderboardDisabled.value = true
      board.value = null
      inviteBoard.value = null
      return
    }
    leaderboardDisabled.value = false
    if (boardType.value === 'invite') {
      inviteBoard.value = await getInviteLeaderboard(activeWindow.value, 20)
      board.value = null
    } else {
      board.value = await getLeaderboard(activeWindow.value, 20)
      inviteBoard.value = null
    }
  } catch (error) {
    if (extractApiErrorCode(error) === 'LEADERBOARD_DISABLED') {
      leaderboardDisabled.value = true
      board.value = null
      inviteBoard.value = null
    } else {
      appStore.showError(extractApiErrorMessage(error, t('leaderboard.loadFailed')))
    }
  } finally {
    loading.value = false
  }
}
function switchWindow(value: LeaderboardWindow): void {
  if (activeWindow.value === value) return
  activeWindow.value = value
  void loadData()
}
function switchBoardType(value: 'usage' | 'invite'): void {
  if (boardType.value === value) return
  boardType.value = value
  void loadData()
}

onMounted(() => { void loadData() })
</script>

<style scoped>
.seg-btn { @apply rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:text-gray-900 dark:text-dark-300 dark:hover:text-white; }
.seg-btn.active { @apply bg-primary-600 text-white shadow-sm hover:text-white; }
.table-th { @apply whitespace-nowrap px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400; }
.table-td { @apply whitespace-nowrap px-5 py-4 text-sm text-gray-600 dark:text-dark-300; }
.numeric-cell { @apply text-right tabular-nums; }
.me-row { @apply bg-primary-50/70 hover:bg-primary-50 dark:bg-primary-500/10 dark:hover:bg-primary-500/15; box-shadow: inset 3px 0 0 0 theme('colors.primary.500'); }
.rank-badge { @apply inline-flex h-8 w-8 items-center justify-center rounded-md border border-gray-200 bg-white text-sm font-semibold tabular-nums text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200; }
.rank-gold { @apply rounded-full border-amber-300 bg-amber-300 text-amber-950 dark:border-amber-400; }
.rank-silver { @apply rounded-full border-slate-300 bg-slate-200 text-slate-800 dark:border-slate-300; }
.rank-bronze { @apply rounded-full border-orange-300 bg-orange-300 text-orange-950 dark:border-orange-400; }
</style>
