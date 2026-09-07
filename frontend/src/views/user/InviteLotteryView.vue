<template>
  <AppLayout legacy-surface>
    <div class="mx-auto max-w-5xl space-y-6" data-testid="invite-lottery-page">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('inviteActivities.inviteLottery.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ lotteryDescription }}</p>
        </div>
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="loading || spinning"
          @click="loadStatus"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
          <span class="ml-1">{{ t('inviteActivities.common.refresh') }}</span>
        </button>
      </div>

      <div
        v-if="loading"
        class="card px-5 py-20 text-center text-sm text-gray-500 dark:text-dark-400"
        aria-busy="true"
      >
        {{ t('inviteActivities.common.loading') }}
      </div>

      <div
        v-else-if="loadFailed"
        class="card px-5 py-16 text-center text-sm text-gray-500 dark:text-dark-400"
        data-testid="invite-lottery-load-failed"
      >
        <p>{{ t('inviteActivities.common.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary mt-4" @click="loadStatus">
          <Icon name="refresh" size="sm" aria-hidden="true" />
          <span>{{ t('inviteActivities.common.retry') }}</span>
        </button>
      </div>

      <div
        v-else-if="disabled"
        class="card px-5 py-20 text-center text-sm text-gray-500 dark:text-dark-400"
        data-testid="invite-lottery-disabled"
      >
        {{ t('inviteActivities.common.disabled') }}
      </div>

      <div v-else-if="status" class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_340px]">
        <section
          class="lottery-stage relative overflow-hidden rounded-3xl p-5 sm:p-8"
          :class="{ 'is-spinning': spinning, 'self-start': denseLottery }"
        >
          <div class="lottery-stage__glow pointer-events-none absolute inset-0"></div>
          <div class="lottery-stage__stars pointer-events-none absolute inset-0"></div>

          <div class="relative mb-5 flex items-center justify-center">
            <div class="lottery-title">
              <Icon name="sparkles" size="sm" class="text-amber-300" aria-hidden="true" />
              <span>{{ t('inviteActivities.inviteLottery.boardTitle') }}</span>
              <Icon name="sparkles" size="sm" class="text-amber-300" aria-hidden="true" />
            </div>
          </div>

          <div
            class="lottery-board-scroll relative mx-auto w-full"
            :aria-label="t('inviteActivities.inviteLottery.boardTitle')"
            data-testid="invite-lottery-board-scroll"
          >
            <div
              class="lottery-board relative mx-auto w-full"
              :class="{ 'lottery-board--dense': denseLottery }"
            >
              <div class="lottery-frame">
              <span class="lottery-corner lottery-corner--tl"></span>
              <span class="lottery-corner lottery-corner--tr"></span>
              <span class="lottery-corner lottery-corner--bl"></span>
              <span class="lottery-corner lottery-corner--br"></span>

              <div
                class="lottery-grid"
                :style="
                  denseLottery
                    ? { gridTemplateColumns: 'repeat(auto-fit, minmax(82px, 1fr))' }
                    : {
                        gridTemplateColumns: `repeat(${side}, minmax(0, 1fr))`,
                        gridTemplateRows: `repeat(${side}, minmax(0, 1fr))`,
                      }
                "
              >
                <div
                  v-for="cell in ringCells"
                  :key="`cell-${cell.index}`"
                  class="lottery-cell"
                  :class="{
                    'lottery-cell--active': cell.index === current,
                    'lottery-cell--winner': cell.index === wonIndex && !spinning,
                    'lottery-cell--filler': cell.filler,
                  }"
                  :style="denseLottery ? undefined : { gridColumn: cell.col, gridRow: cell.row }"
                >
                  <Icon
                    :name="cell.filler ? 'x' : 'gift'"
                    size="sm"
                    class="lottery-cell__icon"
                    aria-hidden="true"
                  />
                  <span class="lottery-cell__name">{{ cell.name }}</span>
                  <span v-if="!cell.filler" class="lottery-cell__amount">${{ money(cell.amount) }}</span>
                </div>

                <div class="lottery-center">
                  <button
                    type="button"
                    class="lottery-start"
                    :class="{
                      'lottery-start--ready': canStart,
                      'lottery-start--spinning': spinning,
                    }"
                    :disabled="!canStart"
                    data-testid="invite-lottery-claim"
                    @click="start"
                  >
                    <Icon name="sparkles" size="lg" aria-hidden="true" />
                    <span class="mt-1 text-base font-extrabold tracking-wide">{{ actionText }}</span>
                    <span v-if="status.enabled && !spinning" class="mt-0.5 text-xs font-medium opacity-90">
                      {{ t('inviteActivities.inviteLottery.remainingShort') }}
                      {{ status.remaining_chances }} {{ t('inviteActivities.common.chances') }}
                    </span>
                  </button>
                </div>
              </div>
            </div>
            </div>
          </div>

          <transition name="prize-pop">
            <div
              v-if="showResult && lastPrize"
              class="lottery-result mx-auto max-w-[560px]"
              role="status"
              data-testid="invite-lottery-result"
            >
              <Icon name="sparkles" size="sm" class="text-amber-500" aria-hidden="true" />
              <span>
                {{ t('inviteActivities.inviteLottery.wonPrefix') }}<b>{{ lastPrize.name }}</b
                >{{ t('inviteActivities.inviteLottery.wonReward')
                }}<b class="text-emerald-600">${{ money(lastPrize.amount) }}</b>
              </span>
            </div>
          </transition>
        </section>

        <aside class="space-y-4">
          <section class="card overflow-hidden p-0">
            <div class="bg-gradient-to-br from-primary-500 to-primary-700 px-5 py-5 text-white">
              <p class="text-xs font-medium uppercase tracking-wide text-white/80">
                {{ t('inviteActivities.inviteLottery.remaining') }}
              </p>
              <div class="mt-1 flex items-end gap-2">
                <span class="text-4xl font-extrabold leading-none">{{ status.remaining_chances }}</span>
                <span class="mb-1 text-sm text-white/80">{{ t('inviteActivities.common.chances') }}</span>
              </div>
            </div>
            <div class="space-y-3 p-5 text-sm">
              <div class="flex items-center justify-between">
                <span class="flex items-center gap-2 text-gray-500 dark:text-dark-400">
                  <Icon name="users" size="sm" class="text-primary-500" aria-hidden="true" />
                  {{ t('inviteActivities.inviteLottery.invited') }}
                </span>
                <span class="font-semibold text-gray-900 dark:text-white">{{ status.invited_count }}</span>
              </div>
              <div v-if="status.recharge_limit_enabled" class="flex items-center justify-between">
                <span class="flex items-center gap-2 text-gray-500 dark:text-dark-400">
                  <Icon name="check" size="sm" class="text-emerald-500" aria-hidden="true" />
                  {{ t('inviteActivities.inviteLottery.qualified') }}
                </span>
                <span class="font-semibold text-gray-900 dark:text-white">
                  {{ status.qualified_invited_count }}
                </span>
              </div>
              <div class="flex items-center justify-between">
                <span class="flex items-center gap-2 text-gray-500 dark:text-dark-400">
                  <Icon name="gift" size="sm" class="text-primary-500" aria-hidden="true" />
                  {{ t('inviteActivities.inviteLottery.used') }}
                </span>
                <span class="font-semibold text-gray-900 dark:text-white">{{ status.used_chances }}</span>
              </div>
              <div
                v-if="status.recharge_limit_enabled && status.locked_chances > 0"
                class="flex items-center justify-between"
              >
                <span class="flex items-center gap-2 text-gray-500 dark:text-dark-400">
                  <Icon name="lock" size="sm" class="text-amber-500" aria-hidden="true" />
                  {{ t('inviteActivities.inviteLottery.locked') }}
                </span>
                <span class="font-semibold text-amber-600 dark:text-amber-400">
                  {{ status.locked_chances }}
                </span>
              </div>
              <p
                v-if="status.recharge_limit_enabled"
                class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-500/10 dark:text-amber-300"
              >
                {{
                  t('inviteActivities.inviteLottery.lockedHint', {
                    amount: money(status.invitee_recharge_threshold),
                  })
                }}
              </p>
              <p
                class="rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-500 dark:bg-dark-800/70 dark:text-dark-400"
              >
                {{ statusText }}
              </p>
              <button type="button" class="btn btn-primary w-full" @click="goAffiliate">
                <Icon name="users" size="sm" aria-hidden="true" />
                <span class="ml-1">{{ t('inviteActivities.inviteLottery.inviteFriends') }}</span>
              </button>
            </div>
          </section>

          <section class="card p-5">
            <h2 class="flex items-center gap-2 text-base font-semibold text-gray-900 dark:text-white">
              <Icon name="gift" size="sm" class="text-amber-500" aria-hidden="true" />
              {{ t('inviteActivities.inviteLottery.prizePool') }}
            </h2>
            <div class="mt-4 space-y-2">
              <div
                v-for="(prize, index) in prizes"
                :key="`prize-${prize.name}-${index}`"
                class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2.5 text-sm transition-colors hover:bg-gray-100 dark:bg-dark-800/70 dark:hover:bg-dark-800"
              >
                <div class="flex min-w-0 items-center gap-2.5">
                  <span
                    class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-bold text-white"
                    :style="{ backgroundColor: colors[index % colors.length] }"
                  >
                    {{ index + 1 }}
                  </span>
                  <span class="truncate text-gray-700 dark:text-dark-200">{{ prize.name }}</span>
                </div>
                <span
                  class="shrink-0 rounded-full bg-emerald-50 px-2.5 py-0.5 font-semibold text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400"
                >
                  ${{ money(prize.amount) }}
                </span>
              </div>
              <p v-if="!prizes.length" class="py-4 text-center text-xs text-gray-400">
                {{ t('inviteActivities.inviteLottery.emptyPrizes') }}
              </p>
            </div>
          </section>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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
const loading = ref(false)
const loadFailed = ref(false)
const spinning = ref(false)
const featureDisabled = ref(false)
const status = ref<InviteLotteryStatus | null>(null)
const current = ref(0)
const wonIndex = ref(-1)
const showResult = ref(false)
const lastPrize = ref<InviteLotteryPrizePublic | null>(null)

const colors = ['#6366f1', '#ec4899', '#f59e0b', '#10b981', '#06b6d4', '#8b5cf6', '#ef4444', '#14b8a6']
const disabled = computed(() => featureDisabled.value || status.value?.enabled === false)
const prizes = computed<InviteLotteryPrizePublic[]>(() =>
  status.value?.prizes?.length ? status.value.prizes : [],
)
const side = computed(() => Math.max(3, Math.ceil((prizes.value.length + 4) / 4)))
const denseLottery = computed(() => prizes.value.length > 8)
const ringCells = computed(() => {
  const coordinates = ringCoords(side.value)
  return coordinates.map((coordinate, index) => {
    const prize = prizes.value[index]
    return {
      index,
      row: coordinate.row + 1,
      col: coordinate.col + 1,
      name: prize ? prize.name : t('inviteActivities.inviteLottery.thanksForJoining'),
      amount: prize ? prize.amount : 0,
      filler: !prize,
    }
  })
})

const canStart = computed(
  () =>
    Boolean(status.value?.can_claim) &&
    !spinning.value &&
    !loading.value &&
    (status.value?.remaining_chances || 0) > 0,
)
const actionText = computed(() => {
  if (spinning.value) return t('inviteActivities.inviteLottery.spinning')
  if (!status.value?.enabled) return t('inviteActivities.common.disabled')
  if ((status.value?.locked_chances || 0) > 0 && (status.value?.remaining_chances || 0) <= 0) {
    return t('inviteActivities.inviteLottery.waitingRecharge')
  }
  if ((status.value?.remaining_chances || 0) <= 0) return t('inviteActivities.inviteLottery.noChance')
  return t('inviteActivities.inviteLottery.draw')
})
const statusText = computed(() => {
  if ((status.value?.remaining_chances || 0) > 0) {
    return t('inviteActivities.inviteLottery.readyHint')
  }
  if ((status.value?.locked_chances || 0) > 0) {
    return t('inviteActivities.inviteLottery.waitingRechargeHint', {
      count: status.value?.locked_chances || 0,
    })
  }
  return t('inviteActivities.inviteLottery.inviteHint')
})
const lotteryDescription = computed(() => {
  if (status.value?.recharge_limit_enabled) {
    return t('inviteActivities.inviteLottery.qualifiedDescription', {
      amount: money(status.value.invitee_recharge_threshold),
    })
  }
  return t('inviteActivities.inviteLottery.description')
})

let disposed = false
let timer: number | undefined
let settleMarquee: (() => void) | undefined
const marqueeDuration = 4800

function ringCoords(size: number): Array<{ row: number; col: number }> {
  const coordinates: Array<{ row: number; col: number }> = []
  for (let column = 0; column < size; column += 1) coordinates.push({ row: 0, col: column })
  for (let row = 1; row < size; row += 1) coordinates.push({ row, col: size - 1 })
  for (let column = size - 2; column >= 0; column -= 1) {
    coordinates.push({ row: size - 1, col: column })
  }
  for (let row = size - 2; row >= 1; row -= 1) coordinates.push({ row, col: 0 })
  return coordinates
}

function money(value: number): string {
  return Number(value || 0).toFixed(2)
}

async function publicFlagEnabled(force = false): Promise<boolean> {
  if (force || !appStore.publicSettingsLoaded) {
    const settings = await appStore.fetchPublicSettings(force)
    if (!settings) return false
  }
  return isInviteActivitySettingsEnabled(
    appStore.publicSettingsLoaded,
    appStore.cachedPublicSettings,
    'subnexus_invite_lottery_enabled',
  )
}

async function loadStatus(): Promise<void> {
  if (disposed) return
  loading.value = true
  loadFailed.value = false
  try {
    if (!(await publicFlagEnabled())) {
      if (disposed) return
      featureDisabled.value = true
      status.value = null
      return
    }
    const result = await getInviteLotteryStatus()
    if (disposed) return
    status.value = result
    featureDisabled.value = !result.enabled
  } catch (error) {
    if (disposed) return
    status.value = null
    loadFailed.value = true
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.loadFailed')))
  } finally {
    if (!disposed) loading.value = false
  }
}

async function start(): Promise<void> {
  if (!canStart.value || disposed) return
  spinning.value = true
  showResult.value = false
  lastPrize.value = null
  wonIndex.value = -1

  let enabled = false
  try {
    enabled = await publicFlagEnabled(true)
  } catch {
    enabled = false
  }
  if (disposed) return
  if (!enabled) {
    featureDisabled.value = true
    status.value = null
    spinning.value = false
    return
  }

  try {
    const result = await claimInviteLottery()
    if (disposed) return
    status.value = result
    featureDisabled.value = !result.enabled
    if (!result.prize) return

    const target = findPrizeRingIndex(result.prize)
    await runMarquee(target)
    if (disposed) return
    wonIndex.value = target
    lastPrize.value = result.prize
    showResult.value = true
    appStore.showSuccess(
      t('inviteActivities.inviteLottery.won', {
        name: result.prize.name,
        amount: money(result.prize.amount),
      }),
    )
  } catch (error) {
    if (disposed) return
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.claimFailed')))
    await loadStatus()
  } finally {
    if (!disposed) spinning.value = false
  }
}

function runMarquee(target: number): Promise<void> {
  return new Promise((resolve) => {
    const capacity = ringCells.value.length
    if (disposed || capacity <= 0) {
      resolve()
      return
    }

    const startIndex = current.value
    const totalSteps = 4 * capacity + ((target - startIndex + capacity) % capacity)
    const delays = marqueeDelays(totalSteps)
    let step = 0
    const finish = () => {
      if (timer !== undefined) window.clearTimeout(timer)
      timer = undefined
      settleMarquee = undefined
      resolve()
    }
    settleMarquee = finish

    const tick = () => {
      if (disposed) {
        finish()
        return
      }
      current.value = (current.value + 1) % capacity
      step += 1
      if (step >= totalSteps) {
        current.value = target
        finish()
        return
      }
      timer = window.setTimeout(tick, delays[step])
    }
    timer = window.setTimeout(tick, delays[0])
  })
}

function marqueeDelays(totalSteps: number): number[] {
  const weights = Array.from({ length: totalSteps }, (_, index) => delayFor(index / totalSteps))
  const totalWeight = weights.reduce((sum, weight) => sum + weight, 0)
  return weights.map((weight) => (weight / totalWeight) * marqueeDuration)
}

function delayFor(progress: number): number {
  const minimum = 60
  const initial = 170
  const ending = 460
  if (progress < 0.18) return initial - (initial - minimum) * (progress / 0.18)
  if (progress < 0.72) return minimum
  return minimum + (ending - minimum) * ((progress - 0.72) / 0.28)
}

function findPrizeRingIndex(prize?: InviteLotteryPrizePublic): number {
  if (!prize || !prizes.value.length) return 0
  const index = prizes.value.findIndex(
    (item) => item.name === prize.name && Number(item.amount) === Number(prize.amount),
  )
  return index >= 0 ? index : 0
}

function goAffiliate(): void {
  void router.push('/affiliate')
}

onMounted(() => {
  void loadStatus()
})
onBeforeUnmount(() => {
  disposed = true
  if (timer !== undefined) window.clearTimeout(timer)
  timer = undefined
  settleMarquee?.()
  settleMarquee = undefined
})
</script>

<style scoped>
.card {
  border-color: rgba(229, 231, 235, 0.7);
  border-radius: 0.75rem;
}
html.dark .card { border-color: rgba(51, 65, 85, 0.6); }
.btn { border-radius: 0.5rem; }
.btn-primary {
  background: #0d9488;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
}
.btn-primary:hover { background: #0f766e; box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1); }
html.dark .btn-primary { background: #0d9488; }
html.dark .btn-primary:hover { background: #14b8a6; }

.lottery-stage {
  background:
    radial-gradient(circle at 18% 12%, rgba(129, 140, 248, 0.35), transparent 42%),
    radial-gradient(circle at 85% 88%, rgba(217, 70, 239, 0.32), transparent 45%),
    linear-gradient(160deg, #211a4d 0%, #3b2a8c 48%, #5b1f86 100%);
  box-shadow: 0 26px 60px -28px rgba(76, 29, 149, 0.85);
}
.lottery-stage__glow {
  background: radial-gradient(circle at 50% 30%, rgba(255, 255, 255, 0.16), transparent 55%);
  opacity: 0.7;
}
.lottery-stage__stars {
  background-image:
    radial-gradient(1.5px 1.5px at 12% 22%, rgba(255, 255, 255, 0.7), transparent),
    radial-gradient(1.5px 1.5px at 72% 16%, rgba(255, 255, 255, 0.5), transparent),
    radial-gradient(1.5px 1.5px at 33% 78%, rgba(255, 255, 255, 0.55), transparent),
    radial-gradient(1.5px 1.5px at 88% 64%, rgba(255, 255, 255, 0.5), transparent),
    radial-gradient(1.5px 1.5px at 56% 44%, rgba(255, 255, 255, 0.45), transparent);
  animation: twinkle 3.6s ease-in-out infinite;
}
@keyframes twinkle {
  0%, 100% { opacity: 0.35; }
  50% { opacity: 0.8; }
}

.lottery-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border-radius: 9999px;
  padding: 6px 20px;
  font-size: 15px;
  font-weight: 800;
  letter-spacing: 2px;
  color: #fff7e0;
  background: linear-gradient(180deg, rgba(251, 191, 36, 0.28), rgba(251, 191, 36, 0.08));
  border: 1px solid rgba(253, 224, 71, 0.55);
  box-shadow: 0 6px 18px -8px rgba(251, 191, 36, 0.8), inset 0 1px 0 rgba(255, 255, 255, 0.35);
  text-shadow: 0 1px 6px rgba(0, 0, 0, 0.45);
}

.lottery-frame {
  position: relative;
  border-radius: 26px;
  padding: 10px;
  isolation: isolate;
  background: linear-gradient(145deg, #fde68a 0%, #f59e0b 38%, #b45309 70%, #f59e0b 100%);
  box-shadow:
    0 0 0 1px rgba(253, 224, 71, 0.55),
    0 22px 48px -18px rgba(245, 158, 11, 0.65),
    inset 0 1px 0 rgba(255, 255, 255, 0.55);
}

.lottery-board { max-width: 560px; }
.lottery-board--dense .lottery-grid {
  aspect-ratio: auto;
  gap: 8px;
  padding: 10px;
}
.lottery-board--dense .lottery-cell {
  min-height: 72px;
  gap: 2px;
  border-radius: 10px;
  padding: 6px 4px;
}
.lottery-board--dense .lottery-center {
  position: relative;
  inset: auto;
  grid-column: 1 / -1;
  grid-row: 1;
  min-height: 150px;
}
.lottery-frame::after {
  content: '';
  position: absolute;
  inset: 6px;
  border-radius: 20px;
  background: linear-gradient(160deg, rgba(30, 27, 75, 0.7), rgba(76, 29, 149, 0.6));
  z-index: 0;
}

.lottery-corner {
  position: absolute;
  width: 12px;
  height: 12px;
  border-radius: 9999px;
  background: #fffbe6;
  box-shadow: 0 0 10px 3px rgba(253, 224, 71, 0.9);
  z-index: 3;
  animation: bulb-blink 1.2s ease-in-out infinite;
}
.lottery-corner--tl { top: -2px; left: -2px; }
.lottery-corner--tr { top: -2px; right: -2px; animation-delay: 0.3s; }
.lottery-corner--bl { bottom: -2px; left: -2px; animation-delay: 0.6s; }
.lottery-corner--br { bottom: -2px; right: -2px; animation-delay: 0.9s; }
.is-spinning .lottery-corner { animation-duration: 0.45s; }
@keyframes bulb-blink {
  0%, 100% { opacity: 1; box-shadow: 0 0 10px 3px rgba(253, 224, 71, 0.9); }
  50% { opacity: 0.45; box-shadow: 0 0 4px 1px rgba(253, 224, 71, 0.5); }
}

.lottery-grid {
  position: relative;
  z-index: 1;
  display: grid;
  gap: 14px;
  aspect-ratio: 1 / 1;
  padding: 14px;
  border-radius: 20px;
  background: linear-gradient(160deg, #1b1840, #2c2168);
  box-shadow: inset 0 0 36px rgba(0, 0, 0, 0.45);
}

.lottery-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 5px;
  border-radius: 16px;
  border: 1.5px solid rgba(255, 255, 255, 0.18);
  background: linear-gradient(160deg, rgba(255, 255, 255, 0.16), rgba(255, 255, 255, 0.05));
  padding: 10px 6px;
  text-align: center;
  color: #fff;
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.45);
  transition: transform 0.08s ease, box-shadow 0.08s ease, background 0.08s ease, border-color 0.08s ease;
}
.lottery-cell__icon {
  color: #fcd34d;
  opacity: 0.9;
  transform: scale(1.15);
}
.lottery-cell__name {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 15px;
  font-weight: 700;
  line-height: 1.2;
}
.lottery-cell__amount {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  font-weight: 800;
  color: #fde68a;
}
.lottery-cell--filler { opacity: 0.55; }
.lottery-cell--filler .lottery-cell__icon { color: #cbd5e1; }
.lottery-cell--filler .lottery-cell__name {
  color: #e5e7eb;
  font-weight: 500;
}
.lottery-cell--active {
  background: linear-gradient(160deg, #fcd34d, #f59e0b);
  border-color: #fff7cc;
  color: #4c1d95;
  text-shadow: none;
  transform: scale(1.07);
  box-shadow: 0 0 0 3px rgba(253, 224, 71, 0.55), 0 10px 22px -6px rgba(251, 191, 36, 0.95);
}
.lottery-cell--active .lottery-cell__icon { color: #b45309; opacity: 1; }
.lottery-cell--active .lottery-cell__amount { color: #7c2d12; }
.lottery-cell--winner { animation: winner-pulse 0.9s ease-in-out infinite; }
@keyframes winner-pulse {
  0%, 100% { box-shadow: 0 0 0 3px rgba(253, 224, 71, 0.6), 0 10px 22px -6px rgba(251, 191, 36, 0.95); transform: scale(1.07); }
  50% { box-shadow: 0 0 0 6px rgba(253, 224, 71, 0.85), 0 14px 30px -4px rgba(251, 191, 36, 1); transform: scale(1.12); }
}

.lottery-center {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}
.lottery-start {
  pointer-events: auto;
  display: flex;
  width: clamp(92px, 28%, 136px);
  aspect-ratio: 1 / 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  border-radius: 9999px;
  border: 5px solid #fff;
  background: radial-gradient(circle at 50% 32%, #818cf8, #4338ca 70%);
  color: #fff;
  box-shadow: 0 10px 24px -6px rgba(67, 56, 202, 0.9), inset 0 2px 8px rgba(255, 255, 255, 0.35);
  transition: transform 0.2s ease, box-shadow 0.2s ease, filter 0.2s ease;
}
.lottery-board--dense .lottery-start { width: 136px; }
.lottery-start--ready { animation: start-pulse 1.5s ease-in-out infinite; }
.lottery-start--ready:hover { transform: scale(1.06); }
.lottery-start:disabled {
  cursor: not-allowed;
  filter: grayscale(0.5) brightness(0.92);
  background: radial-gradient(circle at 50% 32%, #9ca3af, #6b7280);
  box-shadow: none;
}
.lottery-start--spinning {
  filter: none;
  animation: start-spin-glow 0.8s ease-in-out infinite;
}
@keyframes start-pulse {
  0%, 100% { box-shadow: 0 10px 24px -6px rgba(67, 56, 202, 0.9), inset 0 2px 8px rgba(255, 255, 255, 0.35); }
  50% { box-shadow: 0 10px 32px 2px rgba(129, 140, 248, 0.95), inset 0 2px 8px rgba(255, 255, 255, 0.45); }
}
@keyframes start-spin-glow {
  0%, 100% { box-shadow: 0 10px 24px -6px rgba(99, 102, 241, 0.9); }
  50% { box-shadow: 0 10px 34px 4px rgba(217, 70, 239, 0.85); }
}

.lottery-result {
  margin-top: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-radius: 9999px;
  background: rgba(255, 255, 255, 0.96);
  padding: 10px 20px;
  font-size: 14px;
  color: #1f2937;
  box-shadow: 0 10px 24px -8px rgba(0, 0, 0, 0.55);
}
.prize-pop-enter-active { transition: all 0.45s cubic-bezier(0.16, 1, 0.3, 1); }
.prize-pop-enter-from {
  opacity: 0;
  transform: scale(0.7) translateY(12px);
}
</style>
