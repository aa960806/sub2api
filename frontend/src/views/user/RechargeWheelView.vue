<template>
  <AppLayout legacy-surface>
    <div class="mx-auto max-w-5xl space-y-6" data-testid="recharge-wheel-page">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('inviteActivities.rechargeWheel.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
            {{ t('inviteActivities.rechargeWheel.description') }}
          </p>
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
        data-testid="recharge-wheel-load-failed"
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
        data-testid="recharge-wheel-disabled"
      >
        {{ t('inviteActivities.common.disabled') }}
      </div>

      <div v-else-if="status" class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_340px]">
        <section
          class="rw-stage relative overflow-hidden rounded-3xl p-5 sm:p-8"
          :class="{
            'is-spinning': spinning,
            'is-shaking': shaking,
            'self-start': denseInnerWheel || denseOuterWheel,
          }"
        >
          <div class="rw-stage__glow pointer-events-none absolute inset-0"></div>
          <div class="rw-stage__stars pointer-events-none absolute inset-0"></div>

          <div class="relative mb-5 flex items-center justify-center">
            <div class="rw-mode">
              <button
                type="button"
                class="rw-mode__btn"
                :class="{ 'rw-mode__btn--on': mode === 'together' }"
                :disabled="spinning"
                @click="mode = 'together'"
              >
                {{ t('inviteActivities.rechargeWheel.spinTogether') }}
              </button>
              <button
                type="button"
                class="rw-mode__btn"
                :class="{ 'rw-mode__btn--on': mode === 'sequential' }"
                :disabled="spinning"
                @click="mode = 'sequential'"
              >
                {{ t('inviteActivities.rechargeWheel.innerFirst') }}
              </button>
            </div>
          </div>

          <div class="relative mx-auto w-full max-w-[500px]">
            <div class="rw-pointer" :class="{ 'rw-pointer--hot': spinning }">
              <div class="rw-pointer__tip"></div>
            </div>

            <div class="rw-wheels relative aspect-square w-full">
              <div
                class="rw-wheel rw-wheel--outer"
                :style="{
                  background: outerBackground,
                  transform: `rotate(${outerRotation}deg)`,
                  transition: outerTransition,
                }"
              >
                <div
                  v-for="(segment, index) in outerSegments"
                  :key="`outer-divider-${index}`"
                  class="rw-divider rw-divider--outer"
                  :style="{ transform: `rotate(${segment.start}deg)` }"
                ></div>
                <div
                  v-for="segment in visibleOuterSegments"
                  :key="`outer-${segment.index}`"
                  class="rw-label rw-label--outer"
                  :class="{ 'rw-label--dense': denseOuterWheel }"
                  :style="{ transform: `rotate(${segment.center}deg)` }"
                >
                  <span class="rw-label__text">×{{ wheelLabelNumber(segment.value, denseOuterWheel) }}</span>
                </div>
              </div>

              <div
                class="rw-wheel rw-wheel--inner"
                :style="{
                  background: innerBackground,
                  transform: `rotate(${innerRotation}deg)`,
                  transition: innerTransition,
                }"
              >
                <div
                  v-for="(segment, index) in innerSegments"
                  :key="`inner-divider-${index}`"
                  class="rw-divider rw-divider--inner"
                  :style="{ transform: `rotate(${segment.start}deg)` }"
                ></div>
                <div
                  v-for="segment in visibleInnerSegments"
                  :key="`inner-${segment.index}`"
                  class="rw-label rw-label--inner"
                  :class="{ 'rw-label--dense': denseInnerWheel }"
                  :style="{ transform: `rotate(${segment.center}deg)` }"
                >
                  <span class="rw-label__text">${{ wheelLabelNumber(segment.value, denseInnerWheel) }}</span>
                </div>
              </div>

              <div class="rw-center">
                <button
                  type="button"
                  class="rw-start"
                  :class="{ 'rw-start--ready': canStart, 'rw-start--spinning': spinning }"
                  :disabled="!canStart"
                  data-testid="recharge-wheel-claim"
                  @click="start"
                >
                  <Icon name="sparkles" size="lg" aria-hidden="true" />
                  <span class="mt-0.5 text-sm font-extrabold tracking-wide">{{ actionText }}</span>
                  <span v-if="status.enabled && !spinning" class="text-[11px] font-medium opacity-90">
                    {{ t('inviteActivities.rechargeWheel.remainingShort') }}
                    {{ status.remaining_chances }} {{ t('inviteActivities.common.chances') }}
                  </span>
                </button>
              </div>
            </div>
          </div>

          <transition name="rw-reveal">
            <div
              v-if="revealStage !== 'idle'"
              class="rw-reveal"
              role="status"
              data-testid="recharge-wheel-result"
            >
              <div class="rw-reveal__card">
                <p class="rw-reveal__title">{{ t('inviteActivities.rechargeWheel.resultTitle') }}</p>
                <div class="rw-reveal__formula">
                  <span class="rw-reveal__amount">${{ money(result?.amount || 0) }}</span>
                  <transition name="rw-pop">
                    <span
                      v-if="revealStage === 'multiplier' || revealStage === 'total'"
                      class="rw-reveal__mult"
                    >
                      × {{ compactNumber(result?.multiplier || 0) }}
                    </span>
                  </transition>
                  <transition name="rw-pop">
                    <span v-if="revealStage === 'total'" class="rw-reveal__eq">=</span>
                  </transition>
                </div>
                <transition name="rw-pop">
                  <div v-if="revealStage === 'total'" class="rw-reveal__total">
                    ${{ money(displayTotal) }}
                  </div>
                </transition>
                <button
                  v-if="revealStage === 'total'"
                  type="button"
                  class="btn btn-primary mt-4"
                  @click="closeReveal"
                >
                  {{ t('inviteActivities.rechargeWheel.acceptReward') }}
                </button>
              </div>
            </div>
          </transition>
        </section>

        <aside class="space-y-4">
          <section class="card overflow-hidden p-0">
            <div class="bg-gradient-to-br from-amber-500 to-orange-600 px-5 py-5 text-white">
              <p class="text-xs font-medium uppercase tracking-wide text-white/80">
                {{ t('inviteActivities.rechargeWheel.remaining') }}
              </p>
              <div class="mt-1 flex items-end gap-2">
                <span class="text-4xl font-extrabold leading-none">{{ status.remaining_chances }}</span>
                <span class="mb-1 text-sm text-white/80">{{ t('inviteActivities.common.chances') }}</span>
              </div>
            </div>
            <div class="space-y-3 p-5 text-sm">
              <div class="flex items-center justify-between">
                <span class="text-gray-500 dark:text-dark-400">
                  {{ t('inviteActivities.rechargeWheel.rechargeProgress') }}
                </span>
                <span class="font-semibold text-gray-900 dark:text-white">
                  ${{ money(status.recharged_amount) }}
                </span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-gray-500 dark:text-dark-400">
                  {{ t('inviteActivities.rechargeWheel.threshold') }}
                </span>
                <span class="font-semibold text-gray-900 dark:text-white">
                  ${{ money(status.threshold) }}
                </span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-gray-500 dark:text-dark-400">
                  {{ t('inviteActivities.rechargeWheel.used') }}
                </span>
                <span class="font-semibold text-gray-900 dark:text-white">{{ status.used_chances }}</span>
              </div>
              <div class="h-2.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div
                  class="h-full rounded-full bg-gradient-to-r from-amber-400 to-orange-500 transition-all duration-500"
                  :style="{ width: `${progress}%` }"
                ></div>
              </div>
              <p class="text-xs text-gray-500 dark:text-dark-400">{{ statusText }}</p>
              <button type="button" class="btn btn-primary w-full" @click="goRecharge">
                <Icon name="plus" size="sm" aria-hidden="true" />
                <span class="ml-1">{{ t('inviteActivities.rechargeWheel.recharge') }}</span>
              </button>
            </div>
          </section>

          <section class="card p-5">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('inviteActivities.rechargeWheel.amountPool') }}
            </h2>
            <div class="mt-3 flex flex-wrap gap-2">
              <span
                v-for="(amount, index) in amounts"
                :key="`amount-${index}`"
                class="rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400"
                data-testid="recharge-wheel-amount-item"
              >
                ${{ money(amount.amount) }}
              </span>
              <span v-if="!amounts.length" class="text-xs text-gray-400">
                {{ t('inviteActivities.rechargeWheel.emptyPool') }}
              </span>
            </div>
            <h2 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">
              {{ t('inviteActivities.rechargeWheel.multiplierPool') }}
            </h2>
            <div class="mt-3 flex flex-wrap gap-2">
              <span
                v-for="(multiplier, index) in multipliers"
                :key="`multiplier-${index}`"
                class="rounded-full bg-indigo-50 px-2.5 py-1 text-xs font-semibold text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-300"
                data-testid="recharge-wheel-multiplier-item"
              >
                ×{{ compactNumber(multiplier.multiplier) }}
              </span>
              <span v-if="!multipliers.length" class="text-xs text-gray-400">
                {{ t('inviteActivities.rechargeWheel.emptyPool') }}
              </span>
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
const loading = ref(false)
const loadFailed = ref(false)
const spinning = ref(false)
const shaking = ref(false)
const featureDisabled = ref(false)
const status = ref<RechargeWheelStatus | null>(null)
const mode = ref<'together' | 'sequential'>('together')
const innerRotation = ref(0)
const outerRotation = ref(0)
const innerTransition = ref('none')
const outerTransition = ref('none')
const result = ref<RechargeWheelResult | null>(null)
const revealStage = ref<'idle' | 'amount' | 'multiplier' | 'total'>('idle')
const displayTotal = ref(0)

const innerColors = ['#10b981', '#06b6d4', '#22c55e', '#14b8a6', '#0ea5e9', '#84cc16', '#2dd4bf', '#34d399']
const outerColors = ['#6366f1', '#8b5cf6', '#a855f7', '#ec4899', '#f43f5e', '#3b82f6', '#7c3aed', '#d946ef']
const disabled = computed(() => featureDisabled.value || status.value?.enabled === false)
const amounts = computed(() => status.value?.amounts ?? [])
const multipliers = computed(() => status.value?.multipliers ?? [])
const innerSegments = computed(() => buildSegments(amounts.value.map((item) => item.amount)))
const outerSegments = computed(() => buildSegments(multipliers.value.map((item) => item.multiplier)))
const visibleInnerSegments = computed(() =>
  sampleWheelSegments(innerSegments.value, 8, result.value?.amount_index),
)
const visibleOuterSegments = computed(() =>
  sampleWheelSegments(outerSegments.value, 10, result.value?.multiplier_index),
)
const denseInnerWheel = computed(() => innerSegments.value.length > 8)
const denseOuterWheel = computed(() => outerSegments.value.length > 10)
const innerBackground = computed(() => wheelBackground(innerSegments.value, innerColors))
const outerBackground = computed(() => wheelBackground(outerSegments.value, outerColors))
const canStart = computed(
  () =>
    Boolean(status.value?.can_claim) &&
    !spinning.value &&
    !loading.value &&
    (status.value?.remaining_chances || 0) > 0,
)
const actionText = computed(() => {
  if (spinning.value) return t('inviteActivities.rechargeWheel.spinning')
  if (!status.value?.enabled) return t('inviteActivities.common.disabled')
  if ((status.value?.remaining_chances || 0) <= 0) {
    return t('inviteActivities.rechargeWheel.rechargeToJoin')
  }
  return t('inviteActivities.rechargeWheel.draw')
})
const progress = computed(() => {
  const cycle = rechargeCycle()
  return cycle.thresholdCents > 0
    ? Math.min(100, Math.round((cycle.progressCents / cycle.thresholdCents) * 100))
    : 0
})
const statusText = computed(() => {
  if ((status.value?.remaining_chances || 0) > 0) {
    return t('inviteActivities.rechargeWheel.readyHint')
  }
  const cycle = rechargeCycle()
  if (cycle.thresholdCents > 0) {
    return t('inviteActivities.rechargeWheel.rechargeRemaining', {
      amount: money((cycle.thresholdCents - cycle.progressCents) / 100),
    })
  }
  return t('inviteActivities.rechargeWheel.earnHint')
})

let disposed = false
const timers = new Map<number, () => void>()
let animationFrame: number | undefined
let settleCountUp: (() => void) | undefined

function money(value: number): string {
  return Number(value || 0).toFixed(2)
}

function moneyShort(value: number): string {
  const number = Number(value || 0)
  if (Number.isInteger(number)) return String(number)
  return number.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
}

function compactNumber(value: number): string {
  return Number(value || 0).toLocaleString(undefined, { maximumFractionDigits: 2 })
}

function wheelLabelNumber(value: number, compact: boolean): string {
  if (!compact) return moneyShort(value)
  return Intl.NumberFormat(undefined, {
    notation: 'compact',
    maximumFractionDigits: 1,
  }).format(Number(value || 0))
}

function rechargeCycle(): { thresholdCents: number; progressCents: number } {
  const thresholdCents = Math.round(Number(status.value?.threshold || 0) * 100)
  if (thresholdCents <= 0) return { thresholdCents: 0, progressCents: 0 }
  const rechargedCents = Math.round(Number(status.value?.recharged_amount || 0) * 100)
  const progressCents = ((rechargedCents % thresholdCents) + thresholdCents) % thresholdCents
  return { thresholdCents, progressCents }
}

function buildSegments(values: number[]): Array<{ value: number; start: number; end: number; center: number }> {
  const list = values.length ? values : [0]
  const slice = 360 / list.length
  return list.map((value, index) => ({
    value,
    start: index * slice,
    end: (index + 1) * slice,
    center: index * slice + slice / 2,
  }))
}

function sampleWheelSegments(
  segments: Array<{ value: number; start: number; end: number; center: number }>,
  limit: number,
  selectedIndex?: number,
): Array<{ value: number; start: number; end: number; center: number; index: number }> {
  if (segments.length <= limit) {
    return segments.map((segment, index) => ({ ...segment, index }))
  }

  const indexes = Array.from({ length: limit }, (_, index) =>
    Math.floor((index * segments.length) / limit),
  )
  if (
    selectedIndex !== undefined &&
    selectedIndex >= 0 &&
    selectedIndex < segments.length &&
    !indexes.includes(selectedIndex)
  ) {
    indexes[indexes.length - 1] = selectedIndex
    indexes.sort((left, right) => left - right)
  }
  return indexes.map((index) => ({ ...segments[index]!, index }))
}

function wheelBackground(
  segments: Array<{ start: number; end: number }>,
  palette: string[],
): string {
  if (!segments.length) return '#e5e7eb'
  const parts = segments.map(
    (segment, index) => `${palette[index % palette.length]} ${segment.start}deg ${segment.end}deg`,
  )
  return `conic-gradient(from 0deg, ${parts.join(', ')})`
}

async function publicFlagEnabled(force = false): Promise<boolean> {
  if (force || !appStore.publicSettingsLoaded) {
    const settings = await appStore.fetchPublicSettings(force)
    if (!settings) return false
  }
  return isInviteActivitySettingsEnabled(
    appStore.publicSettingsLoaded,
    appStore.cachedPublicSettings,
    'subnexus_recharge_wheel_enabled',
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
    const nextStatus = await getRechargeWheelStatus()
    if (disposed) return
    status.value = nextStatus
    featureDisabled.value = !nextStatus.enabled
  } catch (error) {
    if (disposed) return
    status.value = null
    loadFailed.value = true
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.loadFailed')))
  } finally {
    if (!disposed) loading.value = false
  }
}

function sleep(milliseconds: number): Promise<void> {
  return new Promise((resolve) => {
    if (disposed) {
      resolve()
      return
    }
    let timer = 0
    const finish = () => {
      timers.delete(timer)
      resolve()
    }
    timer = window.setTimeout(finish, milliseconds)
    timers.set(timer, finish)
  })
}

async function start(): Promise<void> {
  if (!canStart.value || disposed) return
  spinning.value = true
  revealStage.value = 'idle'
  result.value = null

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
    const response = await claimRechargeWheel()
    if (disposed) return
    status.value = response
    featureDisabled.value = !response.enabled
    if (!response.result) return

    result.value = response.result
    const amountIndex = response.result.amount_index
    const multiplierIndex = response.result.multiplier_index
    if (mode.value === 'together') {
      await Promise.all([
        spinWheel('inner', amountIndex, innerSegments.value.length, 3800),
        spinWheel('outer', multiplierIndex, outerSegments.value.length, 3800),
      ])
    } else {
      await spinWheel('inner', amountIndex, innerSegments.value.length, 3000)
      if (disposed) return
      await spinWheel('outer', multiplierIndex, outerSegments.value.length, 3000)
    }
    if (disposed) return

    await revealResult(response.result)
  } catch (error) {
    if (disposed) return
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.claimFailed')))
    await loadStatus()
  } finally {
    if (!disposed) spinning.value = false
  }
}

function spinWheel(
  wheel: 'inner' | 'outer',
  index: number,
  count: number,
  duration: number,
): Promise<void> {
  const segmentCount = count || 1
  const slice = 360 / segmentCount
  const center = index * slice + slice / 2
  const rotation = wheel === 'inner' ? innerRotation : outerRotation
  const currentRotation = ((rotation.value % 360) + 360) % 360
  const delta = (360 - center - currentRotation + 720) % 360
  const target = rotation.value + 360 * 6 + delta
  if (wheel === 'inner') {
    innerTransition.value = `transform ${duration}ms cubic-bezier(0.16, 1, 0.3, 1)`
    innerRotation.value = target
  } else {
    outerTransition.value = `transform ${duration}ms cubic-bezier(0.16, 1, 0.3, 1)`
    outerRotation.value = target
  }
  return sleep(duration)
}

async function revealResult(reward: RechargeWheelResult): Promise<void> {
  shaking.value = true
  await sleep(280)
  if (disposed) return
  shaking.value = false
  revealStage.value = 'amount'
  await sleep(620)
  if (disposed) return
  revealStage.value = 'multiplier'
  await sleep(620)
  if (disposed) return
  revealStage.value = 'total'
  await countUp(reward.total, 850)
  if (disposed) return
  appStore.showSuccess(
    t('inviteActivities.rechargeWheel.won', {
      amount: money(reward.amount),
      multiplier: compactNumber(reward.multiplier),
      total: money(reward.total),
    }),
  )
}

function countUp(target: number, duration: number): Promise<void> {
  return new Promise((resolve) => {
    if (disposed) {
      resolve()
      return
    }
    const startedAt = performance.now()
    const finish = () => {
      if (animationFrame !== undefined) cancelAnimationFrame(animationFrame)
      animationFrame = undefined
      settleCountUp = undefined
      resolve()
    }
    settleCountUp = finish
    const step = (now: number) => {
      if (disposed) {
        finish()
        return
      }
      const ratio = Math.min(1, (now - startedAt) / duration)
      displayTotal.value = target * (1 - Math.pow(1 - ratio, 3))
      if (ratio < 1) {
        animationFrame = requestAnimationFrame(step)
      } else {
        displayTotal.value = target
        finish()
      }
    }
    animationFrame = requestAnimationFrame(step)
  })
}

function closeReveal(): void {
  revealStage.value = 'idle'
}

function goRecharge(): void {
  void router.push('/purchase')
}

onMounted(() => {
  void loadStatus()
})
onBeforeUnmount(() => {
  disposed = true
  for (const [timer, finish] of [...timers.entries()]) {
    window.clearTimeout(timer)
    finish()
  }
  timers.clear()
  if (animationFrame !== undefined) cancelAnimationFrame(animationFrame)
  animationFrame = undefined
  settleCountUp?.()
  settleCountUp = undefined
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

.rw-stage {
  background:
    radial-gradient(circle at 16% 12%, rgba(251, 191, 36, 0.28), transparent 42%),
    radial-gradient(circle at 86% 88%, rgba(168, 85, 247, 0.3), transparent 46%),
    linear-gradient(160deg, #2a1346 0%, #4a1d6e 48%, #7c1d4e 100%);
  box-shadow: 0 26px 60px -28px rgba(124, 29, 78, 0.9);
}
.rw-stage__glow { background: radial-gradient(circle at 50% 28%, rgba(255, 255, 255, 0.16), transparent 55%); }
.rw-stage__stars {
  background-image:
    radial-gradient(1.5px 1.5px at 18% 24%, rgba(255, 255, 255, 0.7), transparent),
    radial-gradient(1.5px 1.5px at 74% 18%, rgba(255, 255, 255, 0.5), transparent),
    radial-gradient(1.5px 1.5px at 38% 80%, rgba(255, 255, 255, 0.55), transparent),
    radial-gradient(1.5px 1.5px at 88% 62%, rgba(255, 255, 255, 0.5), transparent);
  animation: rw-twinkle 3.4s ease-in-out infinite;
}
@keyframes rw-twinkle { 0%, 100% { opacity: 0.35; } 50% { opacity: 0.8; } }
.is-shaking { animation: rw-shake 0.28s ease-in-out 2; }
@keyframes rw-shake {
  0%, 100% { transform: translateX(0); }
  25% { transform: translateX(-5px); }
  75% { transform: translateX(5px); }
}

.rw-mode {
  display: inline-flex;
  padding: 4px;
  border-radius: 9999px;
  background: rgba(0, 0, 0, 0.28);
  border: 1px solid rgba(255, 255, 255, 0.18);
}
.rw-mode__btn {
  padding: 5px 16px;
  border-radius: 9999px;
  font-size: 13px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.75);
  transition: all 0.2s ease;
}
.rw-mode__btn--on {
  background: linear-gradient(180deg, #fbbf24, #f59e0b);
  color: #4c1d95;
  box-shadow: 0 4px 12px -4px rgba(251, 191, 36, 0.9);
}
.rw-mode__btn:disabled { cursor: not-allowed; opacity: 0.7; }

.rw-pointer {
  position: absolute;
  left: 50%;
  top: -2px;
  z-index: 30;
  transform: translateX(-50%);
  filter: drop-shadow(0 4px 6px rgba(0, 0, 0, 0.45));
}
.rw-pointer__tip {
  width: 0;
  height: 0;
  border-left: 15px solid transparent;
  border-right: 15px solid transparent;
  border-top: 30px solid #fde047;
}
.rw-pointer--hot .rw-pointer__tip { animation: rw-pointer-flash 0.4s ease-in-out infinite; }
@keyframes rw-pointer-flash {
  0%, 100% { border-top-color: #fde047; }
  50% { border-top-color: #fb7185; }
}

.rw-wheels { filter: drop-shadow(0 18px 32px rgba(0, 0, 0, 0.5)); }
.rw-wheel {
  position: absolute;
  border-radius: 9999px;
  will-change: transform;
}
.rw-wheel--outer {
  inset: 0;
  border: 8px solid #fde68a;
  box-shadow: inset 0 0 0 3px rgba(255, 255, 255, 0.3), inset 0 0 36px rgba(0, 0, 0, 0.3);
}
.rw-wheel--inner {
  inset: 22%;
  border: 6px solid #fff7cc;
  box-shadow: inset 0 0 0 3px rgba(255, 255, 255, 0.4), 0 4px 16px rgba(0, 0, 0, 0.35);
}

.rw-divider {
  position: absolute;
  left: 50%;
  top: 0;
  width: 2px;
  height: 50%;
  margin-left: -1px;
  transform-origin: bottom center;
  background: linear-gradient(to bottom, rgba(255, 255, 255, 0.55), rgba(255, 255, 255, 0.05));
  pointer-events: none;
}
.rw-divider--inner { background: linear-gradient(to bottom, rgba(6, 40, 31, 0.35), rgba(6, 40, 31, 0.04)); }

.rw-label {
  position: absolute;
  inset: 0;
  display: flex;
  justify-content: center;
  pointer-events: none;
}
.rw-label__text {
  white-space: nowrap;
  font-weight: 800;
  letter-spacing: 0;
  color: #fff;
  text-shadow: 0 1px 3px rgba(0, 0, 0, 0.6);
}
.rw-label--outer .rw-label__text { margin-top: 11%; font-size: 17px; }
.rw-label--inner .rw-label__text {
  margin-top: 15%;
  font-size: 14px;
  color: #06281f;
  text-shadow: 0 1px 2px rgba(255, 255, 255, 0.45);
}
.rw-label--outer.rw-label--dense .rw-label__text { font-size: 14px; }
.rw-label--inner.rw-label--dense .rw-label__text { font-size: 12px; }

.rw-center {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 25;
  pointer-events: none;
}
.rw-start {
  pointer-events: auto;
  display: flex;
  height: 108px;
  width: 108px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 4px;
  text-align: center;
  border-radius: 9999px;
  border: 5px solid #fff;
  background: radial-gradient(circle at 50% 32%, #f472b6, #be185d 72%);
  color: #fff;
  box-shadow: 0 8px 22px -6px rgba(190, 24, 93, 0.9), inset 0 2px 8px rgba(255, 255, 255, 0.35);
  transition: transform 0.2s ease, box-shadow 0.2s ease, filter 0.2s ease;
}
.rw-start > span {
  max-width: 92px;
  overflow-wrap: anywhere;
  letter-spacing: 0;
  line-height: 1.15;
}
.rw-start--ready { animation: rw-pulse 1.5s ease-in-out infinite; }
.rw-start--ready:hover { transform: scale(1.06); }
.rw-start:disabled {
  cursor: not-allowed;
  filter: grayscale(0.5) brightness(0.92);
  background: radial-gradient(circle at 50% 32%, #9ca3af, #6b7280);
  box-shadow: none;
}
.rw-start--spinning { filter: none; animation: rw-spin-glow 0.8s ease-in-out infinite; }
@keyframes rw-pulse {
  0%, 100% { box-shadow: 0 8px 22px -6px rgba(190, 24, 93, 0.9), inset 0 2px 8px rgba(255, 255, 255, 0.35); }
  50% { box-shadow: 0 8px 30px 2px rgba(244, 114, 182, 0.95), inset 0 2px 8px rgba(255, 255, 255, 0.45); }
}
@keyframes rw-spin-glow {
  0%, 100% { box-shadow: 0 8px 22px -6px rgba(190, 24, 93, 0.9); }
  50% { box-shadow: 0 8px 32px 4px rgba(168, 85, 247, 0.85); }
}

.rw-reveal {
  position: absolute;
  inset: 0;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(20, 8, 40, 0.55);
  backdrop-filter: blur(2px);
}
.rw-reveal__card {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 26px 34px;
  border-radius: 22px;
  background: linear-gradient(160deg, rgba(255, 255, 255, 0.97), rgba(255, 247, 237, 0.97));
  box-shadow: 0 24px 60px -20px rgba(0, 0, 0, 0.7);
  border: 2px solid rgba(251, 191, 36, 0.6);
}
.rw-reveal__title { font-size: 14px; font-weight: 700; letter-spacing: 3px; color: #b45309; }
.rw-reveal__formula {
  margin-top: 10px;
  display: flex;
  align-items: baseline;
  gap: 10px;
  font-weight: 800;
}
.rw-reveal__amount { font-size: 26px; color: #059669; }
.rw-reveal__mult { font-size: 22px; color: #7c3aed; }
.rw-reveal__eq { font-size: 22px; color: #6b7280; }
.rw-reveal__total {
  margin-top: 8px;
  font-size: 44px;
  font-weight: 900;
  background: linear-gradient(90deg, #f59e0b, #ef4444, #ec4899);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  animation: rw-total-pop 0.5s cubic-bezier(0.16, 1, 0.3, 1);
}
@keyframes rw-total-pop {
  0% { transform: scale(0.6); opacity: 0; }
  100% { transform: scale(1); opacity: 1; }
}
.rw-reveal-enter-active { transition: opacity 0.3s ease; }
.rw-reveal-enter-from { opacity: 0; }
.rw-pop-enter-active { transition: all 0.35s cubic-bezier(0.16, 1, 0.3, 1); }
.rw-pop-enter-from { opacity: 0; transform: scale(0.5) translateY(6px); }
</style>
