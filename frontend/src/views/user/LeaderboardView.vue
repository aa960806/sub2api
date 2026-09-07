<template>
  <AppLayout legacy-surface>
    <section class="space-y-6" data-testid="leaderboard-page">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ boardType === 'invite' ? t('leaderboard.inviteTitle') : t('leaderboard.usageTitle') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
            {{ boardType === 'invite' ? t('leaderboard.inviteDescription') : t('leaderboard.usageDescription') }}
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <div class="leaderboard-segment inline-flex rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900" role="tablist">
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
          <div class="leaderboard-segment inline-flex rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900" role="tablist">
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
            class="btn btn-secondary"
            :disabled="loading"
            :title="t('leaderboard.refresh')"
            @click="loadData"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
            <span class="ml-2">{{ t('leaderboard.refresh') }}</span>
          </button>
        </div>
      </div>

      <div v-if="boardType === 'usage'" class="grid gap-4 lg:grid-cols-3">
        <div class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.totalUsage') }}</p>
          <p class="mt-3 text-2xl font-bold text-gray-900 dark:text-white">${{ formatMoney(board?.total_usage) }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ periodText(board) }}</p>
        </div>
        <div class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.totalRequests') }}</p>
          <p class="mt-3 text-2xl font-bold text-gray-900 dark:text-white">{{ formatCount(board?.requests) }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('leaderboard.periodHint') }}</p>
        </div>
        <div class="card p-5">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.totalTokens') }}</p>
          <p class="mt-3 text-2xl font-bold text-gray-900 dark:text-white">{{ formatToken(board?.tokens) }}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('leaderboard.tokenHint') }}</p>
        </div>
      </div>
      <div v-else class="card p-5">
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.totalInvites') }}</p>
        <p class="mt-3 text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('leaderboard.inviteTotalValue', { count: formatCount(inviteBoard?.total_invites) }) }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ periodText(inviteBoard) }}</p>
      </div>

      <div
        v-if="boardType === 'usage' && !loading && !leaderboardDisabled && !loadError"
        class="card flex items-center gap-4 border-l-4 border-primary-500 p-5"
        data-testid="leaderboard-my-rank"
      >
        <span class="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-300">
          <Icon name="chartBar" size="md" aria-hidden="true" />
        </span>
        <div v-if="myEntry" class="min-w-0">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.myRank') }}</p>
          <p class="mt-0.5 text-lg font-bold text-gray-900 dark:text-white">
            {{ t('leaderboard.rankValue', { rank: myEntry.rank }) }}
            <span class="ml-2 text-sm font-normal text-gray-500 dark:text-dark-400">
              {{ t('leaderboard.myUsageSummary', {
                usage: formatMoney(myEntry.usage),
                requests: formatCount(myEntry.requests),
              }) }}
            </span>
          </p>
        </div>
        <div v-else class="min-w-0">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.myRank') }}</p>
          <p class="mt-0.5 text-lg font-bold text-gray-900 dark:text-white">
            {{ t('leaderboard.notRanked') }}
            <span class="ml-2 text-sm font-normal text-gray-500 dark:text-dark-400">{{ t('leaderboard.notRankedHint') }}</span>
          </p>
        </div>
      </div>

      <div v-if="boardType === 'usage'" class="card overflow-hidden">
        <div class="flex flex-col gap-2 border-b border-gray-200 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('leaderboard.topUsers') }}</h2>
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ periodText(board) }}</p>
          </div>
          <p v-if="rewardHint" class="text-sm font-medium text-amber-600 dark:text-amber-300">{{ rewardHint }}</p>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[900px] table-fixed divide-y divide-gray-200 dark:divide-dark-700">
            <colgroup>
              <col class="w-20" />
              <col class="w-[32%]" />
              <col class="w-36" />
              <col class="w-32" />
              <col class="w-36" />
              <col class="w-32" />
            </colgroup>
            <thead class="bg-gray-50 dark:bg-dark-800/80">
              <tr>
                <th class="table-th text-center">{{ t('leaderboard.rank') }}</th>
                <th class="table-th">{{ t('leaderboard.user') }}</th>
                <th class="table-th numeric-cell">{{ t('leaderboard.usageAmount') }}</th>
                <th class="table-th numeric-cell">{{ t('leaderboard.requests') }}</th>
                <th class="table-th numeric-cell">{{ t('leaderboard.tokens') }}</th>
                <th class="table-th numeric-cell">{{ t('leaderboard.reward') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-if="loading">
                <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('common.loading') }}</td>
              </tr>
              <tr v-else-if="leaderboardDisabled" data-testid="leaderboard-disabled">
                <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.disabled') }}</td>
              </tr>
              <tr v-else-if="loadError" data-testid="leaderboard-load-error">
                <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">
                  <div class="flex flex-col items-center gap-3">
                    <span>{{ loadError }}</span>
                    <button type="button" class="btn btn-secondary" @click="loadData">{{ t('leaderboard.retry') }}</button>
                  </div>
                </td>
              </tr>
              <tr v-else-if="!entries.length">
                <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.empty') }}</td>
              </tr>
              <tr v-for="entry in entries" :key="entry.user_id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60" :class="{ 'me-row': isMe(entry) }">
                <td class="table-td text-center"><span class="rank-badge" :class="rankClass(entry.rank)">{{ entry.rank }}</span></td>
                <td class="table-td">
                  <span class="flex items-center gap-2">
                    <span class="block truncate font-medium text-gray-900 dark:text-white" :title="entry.email">{{ entry.email }}</span>
                    <span v-if="isMe(entry)" class="badge badge-primary shrink-0">{{ t('leaderboard.me') }}</span>
                  </span>
                </td>
                <td class="table-td numeric-cell font-semibold text-gray-900 dark:text-white">${{ formatMoney(entry.usage) }}</td>
                <td class="table-td numeric-cell">{{ formatCount(entry.requests) }}</td>
                <td class="table-td numeric-cell">{{ formatToken(entry.tokens) }}</td>
                <td class="table-td numeric-cell">
                  <span v-if="entry.reward_amount" class="badge badge-warning">
                    ${{ formatMoney(entry.reward_amount) }}<span v-if="entry.rewarded"> {{ t('leaderboard.rewarded') }}</span>
                  </span>
                  <span v-else class="text-gray-400">-</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div v-else class="card overflow-hidden">
        <div class="flex flex-col gap-2 border-b border-gray-200 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('leaderboard.topInviters') }}</h2>
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ periodText(inviteBoard) }}</p>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[520px] table-fixed divide-y divide-gray-200 dark:divide-dark-700">
            <colgroup>
              <col class="w-20" />
              <col />
              <col class="w-40" />
            </colgroup>
            <thead class="bg-gray-50 dark:bg-dark-800/80">
              <tr>
                <th class="table-th text-center">{{ t('leaderboard.rank') }}</th>
                <th class="table-th">{{ t('leaderboard.user') }}</th>
                <th class="table-th numeric-cell">{{ t('leaderboard.inviteCount') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-if="loading">
                <td colspan="3" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('common.loading') }}</td>
              </tr>
              <tr v-else-if="leaderboardDisabled" data-testid="leaderboard-disabled">
                <td colspan="3" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.disabled') }}</td>
              </tr>
              <tr v-else-if="loadError" data-testid="leaderboard-load-error">
                <td colspan="3" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">
                  <div class="flex flex-col items-center gap-3">
                    <span>{{ loadError }}</span>
                    <button type="button" class="btn btn-secondary" @click="loadData">{{ t('leaderboard.retry') }}</button>
                  </div>
                </td>
              </tr>
              <tr v-else-if="!inviteEntries.length">
                <td colspan="3" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('leaderboard.empty') }}</td>
              </tr>
              <tr v-for="entry in inviteEntries" :key="entry.user_id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60" :class="{ 'me-row': isMe(entry) }">
                <td class="table-td text-center"><span class="rank-badge" :class="rankClass(entry.rank)">{{ entry.rank }}</span></td>
                <td class="table-td">
                  <span class="flex items-center gap-2">
                    <span class="block truncate font-medium text-gray-900 dark:text-white" :title="entry.email">{{ entry.email }}</span>
                    <span v-if="isMe(entry)" class="badge badge-primary shrink-0">{{ t('leaderboard.me') }}</span>
                  </span>
                </td>
                <td class="table-td numeric-cell font-semibold text-gray-900 dark:text-white">{{ formatCount(entry.invite_count) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
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

const { locale, t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const activeWindow = ref<LeaderboardWindow>('week')
const boardType = ref<'usage' | 'invite'>('usage')
const board = ref<LeaderboardResponse | null>(null)
const inviteBoard = ref<InviteLeaderboardResponse | null>(null)
const loading = ref(false)
const leaderboardDisabled = ref(false)
const loadError = ref('')
let loadSequence = 0

const windowOptions = computed(() => [
  { value: 'today' as const, label: t('leaderboard.today') },
  { value: 'week' as const, label: t('leaderboard.week') },
  { value: 'month' as const, label: t('leaderboard.month') },
])
const boardTypeOptions = computed(() => [
  { value: 'usage' as const, label: t('leaderboard.usage') },
  { value: 'invite' as const, label: t('leaderboard.invites') },
])
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
  return rewards ? t('leaderboard.rewardHint', {
    period: activeWindow.value === 'week' ? t('leaderboard.weekBoard') : t('leaderboard.monthBoard'),
    rewards,
  }) : ''
})

function periodText(value: { start_date?: string; end_date?: string } | null | undefined): string {
  return value?.start_date && value?.end_date ? t('leaderboard.period', { start: value.start_date, end: value.end_date }) : ''
}
function formatMoney(value: number | undefined): string { return Number(value ?? 0).toFixed(2) }
function formatCount(value: number | undefined): string { return Number(value ?? 0).toLocaleString() }
function formatToken(value: number | undefined): string {
  const number = Number(value ?? 0)
  if (locale.value.toLowerCase().startsWith('zh')) {
    if (number >= 100000000) return `${(number / 100000000).toFixed(1)}亿`
    if (number >= 10000) return `${(number / 10000).toFixed(1)}万`
    return number.toLocaleString(locale.value)
  }
  return new Intl.NumberFormat(locale.value, {
    notation: number >= 10000 ? 'compact' : 'standard',
    maximumFractionDigits: 1,
  }).format(number)
}
function rankClass(rank: number): string {
  return rank === 1 ? 'rank-gold' : rank === 2 ? 'rank-silver' : rank === 3 ? 'rank-bronze' : ''
}
function isMe(entry: { user_id: number }): boolean { return currentUserId.value != null && entry.user_id === currentUserId.value }

async function loadData(): Promise<void> {
  const sequence = ++loadSequence
  const requestedWindow = activeWindow.value
  const requestedType = boardType.value
  loading.value = true
  leaderboardDisabled.value = false
  loadError.value = ''
  board.value = null
  inviteBoard.value = null
  try {
    // Direct component mounts (deep links, stale router state) must obey the
    // same fail-closed flag as the route and must not issue board queries while off.
    if (!appStore.publicSettingsLoaded) await appStore.fetchPublicSettings()
    if (sequence !== loadSequence) return
    if (!isLeaderboardSettingsEnabled(appStore.publicSettingsLoaded, appStore.cachedPublicSettings)) {
      leaderboardDisabled.value = true
      board.value = null
      inviteBoard.value = null
      return
    }
    leaderboardDisabled.value = false
    if (requestedType === 'invite') {
      const result = await getInviteLeaderboard(requestedWindow, 20)
      if (sequence !== loadSequence) return
      inviteBoard.value = result
      board.value = null
    } else {
      const result = await getLeaderboard(requestedWindow, 20)
      if (sequence !== loadSequence) return
      board.value = result
      inviteBoard.value = null
    }
  } catch (error) {
    if (sequence !== loadSequence) return
    if (extractApiErrorCode(error) === 'LEADERBOARD_DISABLED') {
      leaderboardDisabled.value = true
      loadError.value = ''
      board.value = null
      inviteBoard.value = null
    } else {
      board.value = null
      inviteBoard.value = null
      loadError.value = extractApiErrorMessage(error, t('leaderboard.loadFailed'))
      appStore.showError(loadError.value)
    }
  } finally {
    if (sequence === loadSequence) loading.value = false
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
.card {
  border-color: rgb(229 231 235 / 70%);
  border-radius: 0.75rem;
}
.card[data-testid='leaderboard-my-rank'] { border-left-color: theme('colors.primary.500'); }
html.dark .card { border-color: rgb(51 65 85 / 60%); }
html.dark .card[data-testid='leaderboard-my-rank'] { border-left-color: theme('colors.primary.500'); }
.btn { border-radius: 0.5rem; }
.badge-primary { box-shadow: inset 0 0 0 1px rgb(13 148 136 / 10%); }
.badge-warning { box-shadow: inset 0 0 0 1px rgb(217 119 6 / 10%); }
html.dark .badge-primary { box-shadow: inset 0 0 0 1px rgb(45 212 191 / 20%); }
html.dark .badge-warning { box-shadow: inset 0 0 0 1px rgb(251 191 36 / 20%); }
html.dark .leaderboard-segment { background-color: #141d2e; }
.seg-btn { @apply rounded-md px-4 py-2 text-sm font-medium text-gray-600 transition-colors hover:text-gray-900 dark:text-dark-300 dark:hover:text-white; }
.seg-btn.active { @apply bg-primary-600 text-white shadow-sm hover:text-white; }
.table-th { @apply whitespace-nowrap px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400; }
.table-td { @apply whitespace-nowrap px-5 py-4 text-sm text-gray-600 dark:text-dark-300; }
.numeric-cell { @apply text-right tabular-nums; }
.me-row { @apply bg-primary-50/70 hover:bg-primary-50 dark:bg-primary-500/10 dark:hover:bg-primary-500/15; box-shadow: inset 3px 0 0 0 theme('colors.primary.500'); }
.rank-badge { @apply relative inline-flex h-8 w-8 items-center justify-center overflow-hidden rounded-md border border-gray-200 bg-white text-sm font-semibold tabular-nums text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200; }
.rank-gold {
  @apply h-9 w-9 rounded-full border-amber-300 text-amber-950 dark:border-amber-400 dark:text-amber-950;
  background: radial-gradient(circle at 30% 22%, #fff8c9 0, #facc15 34%, #d97706 100%);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.72),
    inset 0 -2px 6px rgba(146, 64, 14, 0.34),
    0 0 0 3px rgba(245, 158, 11, 0.16),
    0 10px 22px -14px rgba(245, 158, 11, 0.9);
}
.rank-silver {
  @apply h-9 w-9 rounded-full border-slate-300 text-slate-800 dark:border-slate-300 dark:text-slate-800;
  background: radial-gradient(circle at 30% 22%, #ffffff 0, #dbe4ee 38%, #94a3b8 100%);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.78),
    inset 0 -2px 6px rgba(51, 65, 85, 0.2),
    0 0 0 3px rgba(148, 163, 184, 0.14),
    0 10px 22px -14px rgba(100, 116, 139, 0.85);
}
.rank-bronze {
  @apply h-9 w-9 rounded-full border-orange-300 text-orange-950 dark:border-orange-400 dark:text-orange-950;
  background: radial-gradient(circle at 30% 22%, #ffedd5 0, #fb923c 38%, #b45309 100%);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.62),
    inset 0 -2px 6px rgba(124, 45, 18, 0.28),
    0 0 0 3px rgba(249, 115, 22, 0.14),
    0 10px 22px -14px rgba(234, 88, 12, 0.86);
}
.rank-gold::after,
.rank-silver::after,
.rank-bronze::after {
  content: "";
  position: absolute;
  left: 22%;
  top: 18%;
  width: 30%;
  height: 30%;
  border-radius: 9999px;
  background: rgba(255, 255, 255, 0.58);
  filter: blur(1px);
}
</style>
