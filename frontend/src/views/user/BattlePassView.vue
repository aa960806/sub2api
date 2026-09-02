<template>
  <AppLayout>
    <div class="space-y-5">
      <div v-if="loading" class="card px-6 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('common.loading') }}</div>

      <template v-else-if="current?.season">
        <section class="bp-summary">
          <div class="min-w-0">
            <p class="bp-summary__eyebrow">{{ seasonEyebrow }}</p>
            <h1>{{ current.season.name }}</h1>
            <p v-if="current.season.description" class="bp-summary__description">{{ current.season.description }}</p>
            <p class="bp-summary__time">{{ formatDate(current.season.start_at) }} - {{ formatDate(current.season.end_at) }} · {{ remainingText }}</p>
          </div>
          <button class="bp-icon-button" type="button" :disabled="refreshing || !featureEnabled" title="刷新战令" aria-label="刷新战令" @click="load">
            <Icon name="refresh" size="sm" :class="refreshing ? 'animate-spin' : ''" />
          </button>
        </section>

        <section class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_300px]">
          <div class="card space-y-4 p-5">
            <div class="flex items-end justify-between gap-4">
              <div>
                <p class="text-xs font-medium text-gray-500 dark:text-dark-400">当前等级</p>
                <p class="mt-1 text-3xl font-semibold text-gray-900 dark:text-white">Lv. {{ current.progress?.level || 1 }}</p>
              </div>
              <p class="text-sm text-gray-600 dark:text-dark-300">{{ current.progress?.exp || 0 }} EXP</p>
            </div>
            <div class="h-2 overflow-hidden rounded bg-gray-100 dark:bg-dark-800">
              <div class="h-full bg-emerald-500 transition-all" :style="{ width: `${levelProgress}%` }"></div>
            </div>
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ nextLevelText }}</p>
          </div>

          <div class="card p-5">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">高级战令</p>
            <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ premiumText }}</p>
            <p class="mt-2 text-sm text-gray-600 dark:text-dark-300">{{ current.season.premium_price }} 余额 · 解锁后可手动领取已达等级的高级奖励</p>
            <button v-if="!current.progress?.premium_unlocked && current.season.runtime_status === 'active'" class="btn btn-primary mt-4 w-full" type="button" :disabled="purchasing || !featureEnabled" @click="purchase">
              {{ purchasing ? '处理中...' : '解锁高级战令' }}
            </button>
          </div>
        </section>

        <section class="card overflow-hidden">
          <header class="bp-rewards-header">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">赛季奖励总览</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">任务经验自动累计；等级达成后奖励需手动领取。拖动或滚轮可浏览全部等级。</p>
            </div>
            <div class="bp-status-legend" aria-label="奖励状态说明">
              <span><i class="bp-dot bp-dot--claimable"></i>可领取</span>
              <span><i class="bp-dot bp-dot--claimed"></i>已领取</span>
              <span><i class="bp-dot bp-dot--locked"></i>未达成</span>
            </div>
          </header>

          <div v-if="rewardLevels.length" class="bp-track-layout">
            <div class="bp-track-labels" aria-hidden="true">
              <div><span class="bp-lane-badge bp-lane-badge--free">免费</span></div>
              <div class="bp-track-labels__level">等级</div>
              <div><span class="bp-lane-badge bp-lane-badge--premium">高级</span></div>
            </div>

            <div class="bp-track-shell">
              <button class="bp-track-nav bp-track-nav--left" type="button" title="向前浏览" aria-label="向前浏览" @click="scrollRewards(-1)">
                <Icon name="chevronLeft" size="sm" />
              </button>
              <div ref="rewardScroller" class="bp-track-scroller" :class="{ 'bp-track-scroller--dragging': dragging }" @wheel="onRewardWheel" @pointerdown="onPointerDown" @pointermove="onPointerMove" @pointerup="onPointerUp" @pointercancel="onPointerUp">
                <div class="bp-track-stage" :style="trackStyle">
                  <div class="bp-track-row">
                    <template v-for="level in rewardLevels" :key="`free-${level}`">
                      <button v-if="rewardFor(level, 'free')" :data-level="level" class="bp-reward-tile" :class="rewardTileClass(rewardFor(level, 'free')!)" type="button" :disabled="!canClaim(rewardFor(level, 'free')!)" :aria-label="rewardAriaLabel(rewardFor(level, 'free')!)" @click="claimOne(rewardFor(level, 'free')!)">
                        <span class="bp-reward-tile__status">{{ rewardStatus(rewardFor(level, 'free')!.status) }}</span>
                        <span class="bp-reward-tile__icon"><Icon :name="rewardIcon(rewardFor(level, 'free')!)" size="lg" /></span>
                        <strong>{{ rewardLabel(rewardFor(level, 'free')!) }}</strong>
                        <small>{{ rewardDescription(rewardFor(level, 'free')!) }}</small>
                      </button>
                      <div v-else class="bp-reward-tile bp-reward-tile--empty"><span>本级无奖励</span></div>
                    </template>
                  </div>

                  <div class="bp-level-track">
                    <div v-for="level in rewardLevels" :key="`level-${level}`" class="bp-level-node" :class="{ 'bp-level-node--reached': level <= currentLevel, 'bp-level-node--current': level === currentLevel }">
                      <span>{{ level }}</span>
                    </div>
                  </div>

                  <div class="bp-track-row">
                    <template v-for="level in rewardLevels" :key="`premium-${level}`">
                      <button v-if="rewardFor(level, 'premium')" :data-level="level" class="bp-reward-tile bp-reward-tile--premium" :class="rewardTileClass(rewardFor(level, 'premium')!)" type="button" :disabled="!canClaim(rewardFor(level, 'premium')!)" :aria-label="rewardAriaLabel(rewardFor(level, 'premium')!)" @click="claimOne(rewardFor(level, 'premium')!)">
                        <span class="bp-reward-tile__status">{{ rewardStatus(rewardFor(level, 'premium')!.status) }}</span>
                        <span class="bp-reward-tile__icon"><Icon :name="rewardIcon(rewardFor(level, 'premium')!)" size="lg" /></span>
                        <strong>{{ rewardLabel(rewardFor(level, 'premium')!) }}</strong>
                        <small>{{ rewardDescription(rewardFor(level, 'premium')!) }}</small>
                      </button>
                      <div v-else class="bp-reward-tile bp-reward-tile--empty"><span>本级无奖励</span></div>
                    </template>
                  </div>
                </div>
              </div>
              <button class="bp-track-nav bp-track-nav--right" type="button" title="向后浏览" aria-label="向后浏览" @click="scrollRewards(1)">
                <Icon name="chevronRight" size="sm" />
              </button>
            </div>
          </div>
          <p v-else class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">暂无奖励配置</p>

          <footer class="bp-rewards-footer">
            <p>{{ rewardFooterText }}</p>
            <button class="btn btn-primary bp-claim-all" type="button" :disabled="!featureEnabled || claimableCount === 0 || claimingAll || current.season.runtime_status !== 'active'" @click="claimAll">
              <Icon name="gift" size="sm" />
              {{ claimingAll ? '领取中...' : '一键领取' }}
            </button>
          </footer>
        </section>

        <section class="grid gap-5 xl:grid-cols-2">
          <div class="card p-5">
            <div class="mb-4 flex items-center justify-between">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">任务进度</h2>
              <div class="flex rounded border border-gray-200 p-0.5 text-xs dark:border-dark-700">
                <button class="bp-tab" :class="{ 'bp-tab--active': taskTab === 'daily' }" type="button" @click="taskTab = 'daily'">每日</button>
                <button class="bp-tab" :class="{ 'bp-tab--active': taskTab === 'season' }" type="button" @click="taskTab = 'season'">赛季</button>
              </div>
            </div>
            <div class="space-y-3">
              <div v-for="task in visibleTasks" :key="task.id" class="bp-task">
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium text-gray-800 dark:text-dark-100">{{ task.name }}</p>
                    <p v-if="task.description" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ task.description }}</p>
                  </div>
                  <span class="shrink-0 text-xs font-medium text-emerald-600 dark:text-emerald-400">+{{ task.exp_reward }} EXP</span>
                </div>
                <div class="mt-2 flex items-center gap-3">
                  <div class="h-1.5 flex-1 overflow-hidden rounded bg-gray-100 dark:bg-dark-800"><div class="h-full bg-emerald-500" :style="{ width: `${taskProgress(task)}%` }"></div></div>
                  <span class="w-24 text-right text-xs text-gray-500 dark:text-dark-400">{{ formatNumber(task.current_value) }} / {{ formatNumber(task.target_value) }}</span>
                </div>
              </div>
              <p v-if="visibleTasks.length === 0" class="py-5 text-center text-sm text-gray-500 dark:text-dark-400">暂无此类任务</p>
            </div>
          </div>

          <div class="card p-5">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">最近记录</h2>
            <div class="mt-3 space-y-2 text-sm">
              <p v-for="item in history.experience.slice(0, 7)" :key="item.id" class="flex justify-between gap-3 text-gray-600 dark:text-dark-300"><span>{{ item.period_key }}</span><span class="font-medium text-emerald-600 dark:text-emerald-400">+{{ item.exp_delta }} EXP</span></p>
              <p v-if="history.experience.length === 0" class="text-gray-500 dark:text-dark-400">尚无经验记录</p>
            </div>
            <div v-if="cosmetics.length" class="mt-5 border-t border-gray-200 pt-4 dark:border-dark-700">
              <p class="text-xs font-medium text-gray-700 dark:text-dark-200">已获得装扮</p>
              <p class="mb-2 mt-1 text-xs text-gray-500 dark:text-dark-400">称号和徽章可分别佩戴一项，仅用于战令身份展示。</p>
              <div class="flex flex-wrap gap-2">
                <button v-for="item in cosmetics" :key="item.id" class="bp-cosmetic" :class="{ 'bp-cosmetic--equipped': item.equipped }" type="button" :disabled="!featureEnabled" @click="equip(item.id)"><span class="bp-cosmetic__kind">{{ item.kind === 'title' ? '称号' : '徽章' }}</span>{{ item.name }}<span v-if="item.equipped"> · 已佩戴</span></button>
              </div>
            </div>
          </div>
        </section>
      </template>

      <div v-else class="card px-6 py-16 text-center">
        <h1 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('battlePass.title') }}</h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ t('battlePass.emptySeason') }}</p>
        <button class="btn btn-secondary mt-5" type="button" @click="router.replace('/activities')">返回活动中心</button>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  battlePassLevelProgress,
  claimAllBattlePassRewards,
  claimBattlePassReward,
  equipBattlePassCosmetic,
  getBattlePassCosmetics,
  getBattlePassCurrent,
  getBattlePassHistory,
  getBattlePassRewards,
  getBattlePassTasks,
  purchaseBattlePass,
  type BattlePassCosmetic,
  type BattlePassCurrent,
  type BattlePassHistory,
  type BattlePassRewardState,
  type BattlePassTaskState,
} from '@/api/battlePass'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
// The route guard is the first line of defence, but a page can outlive a
// settings refresh (or be mounted directly in a test/embedded shell). Keep a
// local, explicit opt-in gate so no Battle Pass data or mutation is requested
// after the rollout switch is known to be off.
const featureEnabled = computed(() =>
  appStore.publicSettingsLoaded === true
  && appStore.cachedPublicSettings?.battle_pass_enabled === true,
)
const loading = ref(true)
const refreshing = ref(false)
const purchasing = ref(false)
const claimingAll = ref(false)
const claimingIDs = ref<Set<number>>(new Set())
const taskTab = ref<'daily' | 'season'>('daily')
const current = ref<BattlePassCurrent | null>(null)
const tasks = ref<BattlePassTaskState[]>([])
const rewards = ref<BattlePassRewardState[]>([])
const cosmetics = ref<BattlePassCosmetic[]>([])
const history = ref<BattlePassHistory>({ experience: [], purchases: [], rewards: [] })
const rewardScroller = ref<HTMLElement | null>(null)
const dragging = ref(false)
let redirecting = false
let pointerStartX = 0
let pointerStartScroll = 0

const visibleTasks = computed(() => tasks.value.filter((task) => task.period_type === taskTab.value))
const rewardLevels = computed(() => [...new Set(rewards.value.map((item) => item.level))].sort((a, b) => a - b))
const currentLevel = computed(() => current.value?.progress?.level || 1)
const claimableCount = computed(() => rewards.value.filter((item) => item.status === 'claimable').length)
const trackStyle = computed(() => ({ '--level-count': Math.max(1, rewardLevels.value.length) }))
const levelProgress = computed(() => battlePassLevelProgress(current.value?.progress))
const nextLevelText = computed(() => {
  const progress = current.value?.progress
  if (!progress) return ''
  if (progress.next_level_exp == null) return '已达到最高等级'
  return `还需 ${Math.max(0, progress.next_level_exp - progress.exp)} EXP 升级`
})
const premiumText = computed(() => current.value?.progress?.premium_unlocked ? '已解锁' : '未解锁')
const seasonEyebrow = computed(() => {
  if (current.value?.season?.runtime_status === 'scheduled') return '即将开始'
  if (current.value?.season?.runtime_status === 'paused') return '已暂停'
  if (current.value?.season?.runtime_status === 'ended') return '已结束'
  return '当前赛季'
})
const remainingText = computed(() => {
  const season = current.value?.season
  if (!season) return ''
  if (season.runtime_status === 'paused') return '赛季已暂停'
  if (season.runtime_status === 'ended') return '赛季已结束'
  const target = season.runtime_status === 'scheduled' ? season.start_at : season.end_at
  const hours = Math.max(0, Math.ceil((new Date(target).getTime() - Date.now()) / 3600000))
  const duration = hours > 48 ? `${Math.ceil(hours / 24)} 天` : `${hours} 小时`
  return season.runtime_status === 'scheduled' ? `距开始 ${duration}` : `剩余 ${duration}`
})
const rewardFooterText = computed(() => {
  if (current.value?.season?.runtime_status === 'scheduled') return '赛季开始后完成任务，即可解锁等级奖励'
  if (current.value?.season?.runtime_status === 'paused') return '赛季已暂停，恢复后可继续完成任务和领取奖励'
  if (current.value?.season?.runtime_status === 'ended') return '赛季已结束，仅可查看奖励与历史记录'
  return claimableCount.value > 0 ? `当前有 ${claimableCount.value} 项奖励可领取` : '继续完成任务，解锁更多赛季奖励'
})

function rewardFor(level: number, track: 'free' | 'premium') { return rewards.value.find((item) => item.level === level && item.track === track) }
function rewardLabel(reward: BattlePassRewardState) {
  const payload = reward.payload || {}
  if (reward.reward_type === 'balance') return `${formatNumber(Number(payload.amount || 0))} 余额`
  if (reward.reward_type === 'concurrency') return `+${formatNumber(Number(payload.amount || 0))} 并发`
  if (reward.reward_type === 'subscription_days') return `${formatNumber(Number(payload.days || 0))} 天订阅`
  return String(payload.name || (reward.reward_type === 'badge' ? '赛季徽章' : reward.reward_type === 'title' ? '限定称号' : '赛季奖励'))
}
function rewardDescription(reward: BattlePassRewardState) {
  if (reward.reward_type === 'balance') return '到账后可用于服务消费'
  if (reward.reward_type === 'concurrency') return '提升账号可用并发额度'
  if (reward.reward_type === 'subscription_days') return '延长指定分组订阅时长'
  if (reward.reward_type === 'badge') return '永久收藏的赛季限定徽章'
  if (reward.reward_type === 'title') return '可在个人装扮中佩戴'
  return '达到等级后即可领取'
}
function rewardStatus(status: BattlePassRewardState['status']) {
  return ({ locked: '未达成', premium_locked: '高级未解锁', claimable: '可领取', pending: '领取中', processing: '领取中', granted: '已领取', granted_capped: '已领取 · 达上限', failed: '发放失败', blocked_config: '配置异常' } as Record<BattlePassRewardState['status'], string>)[status]
}
function rewardIcon(reward: BattlePassRewardState): 'dollar' | 'bolt' | 'calendar' | 'badge' | 'trophy' | 'gift' {
  if (reward.reward_type === 'balance') return 'dollar'
  if (reward.reward_type === 'concurrency') return 'bolt'
  if (reward.reward_type === 'subscription_days') return 'calendar'
  if (reward.reward_type === 'badge') return 'badge'
  if (reward.reward_type === 'title') return 'trophy'
  return 'gift'
}
function rewardTileClass(reward: BattlePassRewardState) { return [`bp-reward-tile--${reward.status}`, { 'bp-reward-tile--busy': claimingIDs.value.has(reward.id) }] }
function rewardAriaLabel(reward: BattlePassRewardState) { return `等级 ${reward.level} ${reward.track === 'free' ? '免费' : '高级'}奖励，${rewardLabel(reward)}，${rewardStatus(reward.status)}` }
function canClaim(reward: BattlePassRewardState) { return featureEnabled.value && reward.status === 'claimable' && !claimingIDs.value.has(reward.id) && !claimingAll.value && current.value?.season?.runtime_status === 'active' }
function taskProgress(task: BattlePassTaskState) { return Math.max(0, Math.min(100, task.current_value / task.target_value * 100)) }
function formatNumber(value: number) { return new Intl.NumberFormat(undefined, { maximumFractionDigits: 4 }).format(value) }
function formatDate(value: string) { return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) }

function redirectToDashboard(): void {
  if (redirecting) return
  redirecting = true
  void router.replace('/dashboard')
}

function clearBattlePassState(): void {
  current.value = null
  tasks.value = []
  rewards.value = []
  history.value = { experience: [], purchases: [], rewards: [] }
  cosmetics.value = []
}

function closeBattlePass(): void {
  clearBattlePassState()
  redirectToDashboard()
}

async function ensureFeatureEnabled(): Promise<boolean> {
  if (!appStore.publicSettingsLoaded) {
    try {
      await appStore.fetchPublicSettings()
    } catch {
      return false
    }
  }
  return featureEnabled.value
}

async function load() {
  refreshing.value = true
  try {
    // Do not even query the authoritative endpoint until the independent
    // public switch has loaded as an explicit true value.
    if (!await ensureFeatureEnabled()) {
      closeBattlePass()
      return
    }

    // The endpoint remains the authoritative, server-side user gate. The
    // public flag check above only prevents avoidable requests in the known
    // closed state.
    const loadedCurrent = await getBattlePassCurrent()
    if (!featureEnabled.value || loadedCurrent.user_side_enabled !== true) {
      closeBattlePass()
      return
    }
    if (!loadedCurrent.season) {
      clearBattlePassState()
      current.value = loadedCurrent
      return
    }
    const [loadedTasks, loadedRewards, loadedHistory, loadedCosmetics] = await Promise.all([getBattlePassTasks(), getBattlePassRewards(), getBattlePassHistory(), getBattlePassCosmetics()])
    if (!featureEnabled.value) {
      closeBattlePass()
      return
    }
    current.value = loadedCurrent
    tasks.value = loadedTasks
    rewards.value = loadedRewards
    history.value = loadedHistory
    cosmetics.value = loadedCosmetics
    await nextTick()
    focusCurrentLevel()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '加载战令失败'))
    closeBattlePass()
  } finally { loading.value = false; refreshing.value = false }
}

async function purchase() {
  if (!featureEnabled.value) { closeBattlePass(); return }
  purchasing.value = true
  try {
    const key = typeof globalThis.crypto?.randomUUID === 'function' ? globalThis.crypto.randomUUID() : `bp-${Date.now()}-${Math.random().toString(36).slice(2)}`
    await purchaseBattlePass(key)
    if (!featureEnabled.value) { closeBattlePass(); return }
    appStore.showSuccess('高级战令已解锁，可领取已达等级的高级奖励')
    await load()
  } catch (error) { appStore.showError(extractApiErrorMessage(error, '解锁失败')) } finally { purchasing.value = false }
}

async function claimOne(reward: BattlePassRewardState) {
  if (!featureEnabled.value) { closeBattlePass(); return }
  if (!canClaim(reward)) return
  claimingIDs.value = new Set([...claimingIDs.value, reward.id])
  try {
    const result = await claimBattlePassReward(reward.id)
    if (!featureEnabled.value) { closeBattlePass(); return }
    rewards.value = result.rewards
    const loadedHistory = await getBattlePassHistory()
    if (!featureEnabled.value) { closeBattlePass(); return }
    history.value = loadedHistory
    appStore.showSuccess(result.claimed_count > 0 ? '奖励已领取' : '该奖励已经领取')
  } catch (error) { appStore.showError(extractApiErrorMessage(error, '领取失败')) } finally {
    const next = new Set(claimingIDs.value); next.delete(reward.id); claimingIDs.value = next
  }
}

async function claimAll() {
  if (!featureEnabled.value) { closeBattlePass(); return }
  if (claimableCount.value === 0 || claimingAll.value) return
  claimingAll.value = true
  try {
    const result = await claimAllBattlePassRewards()
    if (!featureEnabled.value) { closeBattlePass(); return }
    rewards.value = result.rewards
    const loadedHistory = await getBattlePassHistory()
    if (!featureEnabled.value) { closeBattlePass(); return }
    history.value = loadedHistory
    appStore.showSuccess(result.claimed_count > 0 ? `已领取 ${result.claimed_count} 项奖励` : '暂无可领取奖励')
  } catch (error) { appStore.showError(extractApiErrorMessage(error, '一键领取失败')) } finally { claimingAll.value = false }
}

async function equip(cosmeticID: number) {
  if (!featureEnabled.value) { closeBattlePass(); return }
  try {
    await equipBattlePassCosmetic(cosmeticID)
    if (!featureEnabled.value) { closeBattlePass(); return }
    const loadedCosmetics = await getBattlePassCosmetics()
    if (!featureEnabled.value) { closeBattlePass(); return }
    cosmetics.value = loadedCosmetics
  } catch (error) { appStore.showError(extractApiErrorMessage(error, '佩戴失败')) }
}
function focusCurrentLevel() {
  const scroller = rewardScroller.value
  if (!scroller) return
  const levelIndex = Math.max(0, rewardLevels.value.findIndex((level) => level >= currentLevel.value))
  scroller.scrollLeft = Math.max(0, levelIndex * 142 - scroller.clientWidth / 2 + 66)
}
function scrollRewards(direction: -1 | 1) { rewardScroller.value?.scrollBy({ left: direction * Math.max(280, rewardScroller.value.clientWidth * .7), behavior: 'smooth' }) }
function onRewardWheel(event: WheelEvent) {
  const scroller = rewardScroller.value
  if (!scroller || scroller.scrollWidth <= scroller.clientWidth) return
  const delta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY
  if (delta === 0) return
  event.preventDefault(); scroller.scrollLeft += delta
}
function onPointerDown(event: PointerEvent) {
  if (event.button !== 0 || (event.target as HTMLElement).closest('button')) return
  const scroller = rewardScroller.value
  if (!scroller) return
  dragging.value = true; pointerStartX = event.clientX; pointerStartScroll = scroller.scrollLeft; scroller.setPointerCapture(event.pointerId)
}
function onPointerMove(event: PointerEvent) { if (dragging.value && rewardScroller.value) rewardScroller.value.scrollLeft = pointerStartScroll - (event.clientX - pointerStartX) }
function onPointerUp(event: PointerEvent) {
  if (!dragging.value) return
  dragging.value = false
  if (rewardScroller.value?.hasPointerCapture(event.pointerId)) rewardScroller.value.releasePointerCapture(event.pointerId)
}

// A settings update in the same SPA can close the feature while this page is
// open. Remove the in-memory data immediately and leave the protected route.
watch(featureEnabled, (enabled, wasEnabled) => {
  if (wasEnabled !== true || enabled) return
  closeBattlePass()
})

onMounted(load)
</script>

<style scoped>
.bp-summary { display:flex; align-items:flex-start; justify-content:space-between; gap:16px; border-left:4px solid #10b981; background:#f0fdf4; padding:24px; }.dark .bp-summary { background:rgba(6,78,59,.22); }
.bp-summary__eyebrow { margin:0; color:#047857; font-size:12px; font-weight:600; }.bp-summary h1 { margin:5px 0 0; color:#111827; font-size:24px; font-weight:700; }.dark .bp-summary h1 { color:#fff; }
.bp-summary__description,.bp-summary__time { margin:8px 0 0; color:#4b5563; font-size:14px; }.dark .bp-summary__description,.dark .bp-summary__time { color:#cbd5e1; }
.bp-icon-button { display:grid; height:34px; width:34px; flex:none; place-items:center; border:1px solid #a7f3d0; color:#047857; }.bp-icon-button:disabled { cursor:not-allowed; opacity:.6; }
.bp-rewards-header { display:flex; align-items:flex-start; justify-content:space-between; gap:20px; border-bottom:1px solid #e5e7eb; padding:18px 20px; }.dark .bp-rewards-header { border-color:#374151; }
.bp-status-legend { display:flex; flex-wrap:wrap; gap:12px; color:#6b7280; font-size:11px; }.bp-status-legend span { display:inline-flex; align-items:center; gap:5px; white-space:nowrap; }
.bp-dot { height:7px; width:7px; border-radius:50%; background:#9ca3af; }.bp-dot--claimable { background:#10b981; }.bp-dot--claimed { background:#2563eb; }
.bp-track-layout { display:grid; grid-template-columns:72px minmax(0,1fr); min-height:372px; padding:16px 12px 10px; }
.bp-track-labels { display:grid; grid-template-rows:148px 54px 148px; align-items:center; justify-items:center; border-right:1px solid #e5e7eb; }.dark .bp-track-labels { border-color:#374151; }.bp-track-labels__level { color:#9ca3af; font-size:11px; font-weight:600; }
.bp-lane-badge { display:inline-flex; min-width:44px; justify-content:center; border:1px solid; padding:5px 7px; font-size:11px; font-weight:700; }.bp-lane-badge--free { border-color:#a7f3d0; background:#ecfdf5; color:#047857; }.bp-lane-badge--premium { border-color:#fde68a; background:#fffbeb; color:#b45309; }
.dark .bp-lane-badge--free { border-color:rgba(16,185,129,.35); background:rgba(16,185,129,.12); color:#6ee7b7; }.dark .bp-lane-badge--premium { border-color:rgba(245,158,11,.35); background:rgba(245,158,11,.12); color:#fcd34d; }
.bp-track-shell { position:relative; min-width:0; }.bp-track-scroller { height:356px; overflow-x:auto; overflow-y:hidden; padding:0 30px; cursor:grab; scrollbar-width:thin; scrollbar-color:#d1d5db transparent; touch-action:pan-x; }.bp-track-scroller--dragging { cursor:grabbing; user-select:none; }
.bp-track-stage { display:grid; width:max-content; min-width:100%; grid-template-rows:148px 54px 148px; }.bp-track-row,.bp-level-track { display:grid; grid-template-columns:repeat(var(--level-count),132px); gap:10px; }
.bp-reward-tile { position:relative; display:flex; height:138px; min-width:0; flex-direction:column; align-items:center; justify-content:center; border:1px solid #d1d5db; background:#fff; padding:26px 9px 10px; color:#374151; text-align:center; transition:border-color .15s ease,background-color .15s ease,transform .15s ease; }
.dark .bp-reward-tile { border-color:#4b5563; background:#111827; color:#e5e7eb; }.bp-reward-tile__status { position:absolute; left:7px; top:7px; max-width:calc(100% - 14px); overflow:hidden; color:#6b7280; font-size:10px; font-weight:600; text-overflow:ellipsis; white-space:nowrap; }
.bp-reward-tile__icon { display:grid; height:38px; width:38px; place-items:center; border:1px solid #e5e7eb; color:#6b7280; }.dark .bp-reward-tile__icon { border-color:#374151; }.bp-reward-tile strong { display:block; margin-top:8px; max-width:100%; overflow:hidden; font-size:12px; text-overflow:ellipsis; white-space:nowrap; }.bp-reward-tile small { display:-webkit-box; margin-top:4px; max-width:100%; overflow:hidden; color:#9ca3af; font-size:10px; line-height:1.35; -webkit-box-orient:vertical; -webkit-line-clamp:2; }
.bp-reward-tile--premium { border-color:#fde68a; background:#fffdf5; }.dark .bp-reward-tile--premium { border-color:rgba(245,158,11,.35); background:rgba(120,53,15,.12); }
.bp-reward-tile--claimable { cursor:pointer; border-color:#34d399; background:#ecfdf5; color:#047857; box-shadow:inset 0 0 0 1px rgba(16,185,129,.16); }.bp-reward-tile--claimable:hover { border-color:#059669; background:#d1fae5; transform:translateY(-2px); }.dark .bp-reward-tile--claimable { border-color:#10b981; background:rgba(6,95,70,.28); color:#6ee7b7; }
.bp-reward-tile--claimable .bp-reward-tile__status { color:#047857; }.bp-reward-tile--claimable .bp-reward-tile__icon { border-color:#6ee7b7; color:#059669; }
.bp-reward-tile--granted,.bp-reward-tile--granted_capped { border-color:#93c5fd; background:#eff6ff; color:#1d4ed8; }.dark .bp-reward-tile--granted,.dark .bp-reward-tile--granted_capped { border-color:rgba(59,130,246,.38); background:rgba(30,64,175,.16); color:#93c5fd; }.bp-reward-tile--granted .bp-reward-tile__status,.bp-reward-tile--granted_capped .bp-reward-tile__status { color:#2563eb; }
.bp-reward-tile--locked,.bp-reward-tile--premium_locked { filter:grayscale(.35); opacity:.55; }.bp-reward-tile--premium_locked .bp-reward-tile__status { color:#b45309; }.bp-reward-tile--pending,.bp-reward-tile--processing,.bp-reward-tile--busy { border-color:#6ee7b7; opacity:.75; }.bp-reward-tile--failed,.bp-reward-tile--blocked_config { border-color:#fca5a5; background:#fef2f2; }.dark .bp-reward-tile--failed,.dark .bp-reward-tile--blocked_config { background:rgba(127,29,29,.16); }.bp-reward-tile--empty { border-style:dashed; color:#9ca3af; font-size:11px; opacity:.55; }
.bp-level-track { position:relative; align-items:center; }.bp-level-track::before { position:absolute; left:0; right:0; top:50%; height:2px; background:#e5e7eb; content:''; }.dark .bp-level-track::before { background:#374151; }.bp-level-node { position:relative; z-index:1; display:flex; justify-content:center; }.bp-level-node span { display:grid; height:30px; width:30px; place-items:center; border:2px solid #d1d5db; border-radius:50%; background:#fff; color:#6b7280; font-size:11px; font-weight:700; }.dark .bp-level-node span { border-color:#4b5563; background:#111827; color:#9ca3af; }.bp-level-node--reached span { border-color:#10b981; color:#047857; }.bp-level-node--current span { border-color:#059669; background:#059669; color:#fff; box-shadow:0 0 0 4px rgba(16,185,129,.15); }
.bp-track-nav { position:absolute; z-index:3; top:50%; display:grid; height:32px; width:28px; place-items:center; border:1px solid #d1d5db; background:rgba(255,255,255,.94); color:#4b5563; transform:translateY(-50%); }.dark .bp-track-nav { border-color:#4b5563; background:rgba(17,24,39,.94); color:#d1d5db; }.bp-track-nav--left { left:2px; }.bp-track-nav--right { right:2px; }
.bp-rewards-footer { display:flex; align-items:center; justify-content:flex-end; gap:18px; border-top:1px solid #e5e7eb; padding:12px 20px; color:#6b7280; font-size:12px; }.dark .bp-rewards-footer { border-color:#374151; color:#9ca3af; }.bp-claim-all { display:inline-flex; min-width:126px; align-items:center; justify-content:center; gap:7px; }
.bp-tab { min-width:48px; padding:5px 8px; color:#6b7280; }.bp-tab--active { background:#ecfdf5; color:#047857; font-weight:600; }.dark .bp-tab--active { background:rgba(16,185,129,.15); color:#6ee7b7; }.bp-task { border-bottom:1px solid #e5e7eb; padding-bottom:12px; }.dark .bp-task { border-color:#374151; }.bp-task:last-child { border-bottom:0; padding-bottom:0; }
.bp-cosmetic { border:1px solid #d1d5db; padding:7px 10px; font-size:13px; color:#374151; }.dark .bp-cosmetic { border-color:#4b5563; color:#e5e7eb; }.bp-cosmetic--equipped { border-color:#10b981; color:#047857; background:#ecfdf5; }.dark .bp-cosmetic--equipped { color:#6ee7b7; background:rgba(16,185,129,.12); }.bp-cosmetic__kind { margin-right:6px; font-size:10px; font-weight:700; color:#059669; }
@media (max-width:640px) { .bp-summary { padding:18px; }.bp-summary h1 { font-size:20px; }.bp-rewards-header { display:block; }.bp-status-legend { margin-top:12px; }.bp-track-layout { grid-template-columns:56px minmax(0,1fr); padding-inline:6px; }.bp-lane-badge { min-width:38px; padding-inline:4px; }.bp-track-scroller { padding-inline:24px; }.bp-track-row,.bp-level-track { grid-template-columns:repeat(var(--level-count),116px); gap:8px; }.bp-rewards-footer { align-items:stretch; flex-direction:column; gap:8px; }.bp-rewards-footer p { text-align:right; }.bp-claim-all { width:100%; } }
</style>
