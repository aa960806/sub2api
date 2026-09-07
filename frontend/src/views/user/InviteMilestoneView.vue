<template>
  <AppLayout legacy-surface>
    <div class="mx-auto max-w-4xl space-y-6" data-testid="invite-milestone-page">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('inviteActivities.inviteMilestone.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ milestoneDescription }}</p>
        </div>
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="loading || claimingTarget !== null"
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
        data-testid="invite-milestone-load-failed"
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
        data-testid="invite-milestone-disabled"
      >
        {{ t('inviteActivities.common.disabled') }}
      </div>

      <div
        v-else-if="status && !tiers.length"
        class="card px-5 py-20 text-center text-sm text-gray-500 dark:text-dark-400"
      >
        {{ t('inviteActivities.inviteMilestone.empty') }}
      </div>

      <template v-else-if="status">
        <section class="ms-stage relative overflow-hidden rounded-3xl p-6 sm:p-9">
          <div class="pointer-events-none absolute inset-0 ms-stage__glow"></div>
          <div class="pointer-events-none absolute inset-0 ms-stage__stars"></div>

          <div class="relative flex flex-col items-center">
            <p class="text-xs font-medium uppercase tracking-[3px] text-white/70">
              {{ t('inviteActivities.inviteMilestone.invited') }}
            </p>
            <div class="mt-1 flex items-end gap-2 text-white">
              <span class="text-5xl font-extrabold leading-none">{{ invited }}</span>
              <span v-if="isChinese" class="mb-1.5 text-base text-white/80">人</span>
            </div>
            <p class="mt-2 text-sm text-white/85">{{ progressText }}</p>
            <p
              v-if="status.recharge_limit_enabled"
              class="mt-2 rounded-full bg-white/10 px-3 py-1 text-xs font-medium text-white/90"
            >
              <template v-if="isChinese">
                充值达标 {{ qualifiedInvited }} / {{ invited }} 人 · 每人满
                ${{ money(status.invitee_recharge_threshold) }}
              </template>
              <template v-else>
                {{ t('inviteActivities.inviteMilestone.qualified') }} {{ qualifiedInvited }} /
                {{ invited }} ·
                {{
                  t('inviteActivities.inviteMilestone.qualificationHint', {
                    amount: money(status.invitee_recharge_threshold),
                  })
                }}
              </template>
            </p>

            <span v-if="claimableCount > 0" class="ms-badge mt-3">
              <Icon name="gift" size="sm" aria-hidden="true" />
              <template v-if="isChinese">{{ claimableCount }} 个宝箱可领取</template>
              <template v-else>{{ claimableCount }} {{ t('inviteActivities.common.reward') }}</template>
            </span>
          </div>

          <div
            class="ms-track-scroll relative mt-10 px-6 sm:px-9"
            tabindex="0"
            :aria-label="t('inviteActivities.inviteMilestone.progress')"
            data-testid="invite-milestone-track-scroll"
          >
            <div
              class="ms-track-space relative h-32"
              :style="{ minWidth: `${milestoneTrackMinWidth}px` }"
            >
              <div class="ms-track" aria-hidden="true">
                <div
                  class="ms-fill"
                  :style="{
                    width: `${progressPct}%`,
                    '--ms-mobile-width': `${interpolatedProgressPct}%`,
                  }"
                ></div>
              </div>

              <div
                v-for="(tier, index) in tiers"
                :key="`tier-${tier.invites}`"
                class="ms-node"
                :style="{
                  left: `${tierPositionPct(index)}%`,
                  '--ms-mobile-left': `${equidistantTierPositionPct(index)}%`,
                }"
              >
                <span class="ms-node__req">
                  <template v-if="isChinese">{{ tier.invites }}人</template>
                  <template v-else>
                    {{ t('inviteActivities.inviteMilestone.tierTarget', { count: tier.invites }) }}
                  </template>
                </span>
                <button
                  type="button"
                  class="ms-chest"
                  :class="chestClass(tier)"
                  :disabled="!tier.claimable || claimingTarget !== null"
                  :aria-label="tierAction(tier)"
                  :data-testid="`invite-milestone-claim-${tier.invites}`"
                  @click="claimTier(tier)"
                >
                  <Icon
                    :name="tier.claimed ? 'check' : tier.blocked_by_recharge ? 'lock' : 'gift'"
                    size="md"
                    aria-hidden="true"
                  />
                  <span v-if="tier.claimable" class="ms-chest__spark">
                    <Icon name="sparkles" size="xs" aria-hidden="true" />
                  </span>
                </button>
                <span
                  class="ms-node__reward"
                  :class="{ 'ms-node__reward--on': tier.claimable || tier.claimed }"
                >
                  ${{ money(tier.reward) }}
                </span>
              </div>
            </div>
          </div>

          <transition name="ms-reveal">
            <div
              v-if="showReward"
              class="ms-reveal"
              role="status"
              data-testid="invite-milestone-result"
              @click="showReward = false"
            >
              <div class="ms-reveal__card">
                <Icon name="gift" size="xl" class="text-amber-500" aria-hidden="true" />
                <p class="ms-reveal__title">
                  {{ t('inviteActivities.inviteMilestone.revealTitle') }}
                </p>
                <div class="ms-reveal__amount">+${{ money(rewardAmount) }}</div>
                <p class="ms-reveal__hint">
                  {{
                    t('inviteActivities.inviteMilestone.revealHint')
                  }}
                </p>
              </div>
            </div>
          </transition>
        </section>

        <section class="card p-5">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('inviteActivities.inviteMilestone.rewardsTitle') }}
          </h2>
          <div class="mt-4 space-y-2">
            <div
              v-for="tier in tiers"
              :key="`row-${tier.invites}`"
              class="flex items-center justify-between gap-3 rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-800/70"
            >
              <div class="flex min-w-0 items-center gap-3">
                <span
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full"
                  :class="
                    tier.claimed
                      ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-500/15'
                      : tier.claimable
                        ? 'bg-amber-100 text-amber-600 dark:bg-amber-500/15'
                        : 'bg-gray-200 text-gray-400 dark:bg-dark-700'
                  "
                >
                  <Icon :name="tier.claimed ? 'check' : 'gift'" size="sm" aria-hidden="true" />
                </span>
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                    {{ t('inviteActivities.inviteMilestone.tierTarget', { count: tier.invites }) }}
                  </p>
                  <p class="text-xs text-gray-500 dark:text-dark-400">
                    {{ tierLabel(tier) }}
                  </p>
                </div>
              </div>
              <button
                v-if="tier.claimable"
                type="button"
                class="btn btn-primary btn-sm shrink-0"
                :disabled="claimingTarget !== null"
                @click="claimTier(tier)"
              >
                {{ claimingTarget === tier.invites ? t('inviteActivities.common.claiming') : t('inviteActivities.inviteMilestone.claimButton') }}
              </button>
              <span
                v-else-if="tier.claimed"
                class="shrink-0 rounded-full bg-emerald-50 px-3 py-1 text-xs font-semibold text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400"
              >
                {{ t('inviteActivities.common.claimed') }}
              </span>
              <span
                v-else-if="tier.blocked_by_recharge"
                class="shrink-0 rounded-full bg-amber-50 px-3 py-1 text-xs font-semibold text-amber-600 dark:bg-amber-500/10 dark:text-amber-400"
              >
                {{ t('inviteActivities.inviteMilestone.waitingRecharge') }}
              </span>
              <span
                v-else
                class="shrink-0 rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-400 dark:bg-dark-700"
              >
                {{ t('inviteActivities.inviteMilestone.locked') }}
              </span>
            </div>
          </div>
          <button type="button" class="btn btn-primary mt-4 w-full" @click="goAffiliate">
            <Icon name="users" size="sm" aria-hidden="true" />
            <span class="ml-1">{{ t('inviteActivities.inviteMilestone.inviteFriends') }}</span>
          </button>
        </section>
      </template>
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
  claimInviteMilestone,
  getInviteMilestoneStatus,
  type InviteMilestoneStatus,
  type InviteMilestoneTierStatus,
} from '@/api/inviteActivities'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isInviteActivitySettingsEnabled } from '@/utils/inviteActivities'

const { locale, t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const loading = ref(false)
const loadFailed = ref(false)
const claimingTarget = ref<number | null>(null)
const featureDisabled = ref(false)
const status = ref<InviteMilestoneStatus | null>(null)
const showReward = ref(false)
const rewardAmount = ref(0)

const disabled = computed(() => featureDisabled.value || status.value?.enabled === false)
const isChinese = computed(() => locale.value.toLowerCase().startsWith('zh'))
const tiers = computed<InviteMilestoneTierStatus[]>(() => status.value?.tiers ?? [])
const invited = computed(() => status.value?.invited_count ?? 0)
const qualifiedInvited = computed(() => status.value?.qualified_invited_count ?? invited.value)
const denseMilestones = computed(() => tiers.value.length > 8)
const maxInvites = computed(() =>
  tiers.value.length ? Math.max(...tiers.value.map((tier) => tier.invites)) : 0,
)
const milestoneTrackMinWidth = computed(() => Math.max(88, tiers.value.length * 88))
const interpolatedProgressPct = computed(() => {
  const tierCount = tiers.value.length
  if (!tierCount) return 0

  const nextIndex = tiers.value.findIndex((tier) => invited.value < tier.invites)
  if (nextIndex < 0) return 100

  const nextInvites = tiers.value[nextIndex]!.invites
  const previousInvites = nextIndex > 0 ? tiers.value[nextIndex - 1]!.invites : 0
  const inviteRange = nextInvites - previousInvites
  const intervalProgress =
    inviteRange > 0
      ? Math.min(1, Math.max(0, (invited.value - previousInvites) / inviteRange))
      : 1

  return ((nextIndex + intervalProgress) / tierCount) * 100
})
const progressPct = computed(() => {
  if (denseMilestones.value) return interpolatedProgressPct.value
  return maxInvites.value > 0 ? Math.min(100, (invited.value / maxInvites.value) * 100) : 0
})
const claimableCount = computed(() => tiers.value.filter((tier) => tier.claimable).length)
const nextTier = computed(() => tiers.value.find((tier) => invited.value < tier.invites))
const progressText = computed(() => {
  if (claimableCount.value > 0) {
    return t('inviteActivities.inviteMilestone.readyHint')
  }
  const rechargeBlocked = tiers.value.find((tier) => tier.blocked_by_recharge)
  if (rechargeBlocked) {
    return t('inviteActivities.inviteMilestone.waitingRechargeHint', {
      count: Math.max(0, rechargeBlocked.invites - qualifiedInvited.value),
    })
  }
  if (nextTier.value) {
    return t('inviteActivities.inviteMilestone.nextTierHint', {
      count: Math.max(0, nextTier.value.invites - invited.value),
    })
  }
  return t('inviteActivities.inviteMilestone.completeHint')
})
const milestoneDescription = computed(() => {
  if (status.value?.recharge_limit_enabled) {
    return t('inviteActivities.inviteMilestone.qualificationHint', {
      amount: money(status.value.invitee_recharge_threshold),
    })
  }
  return t('inviteActivities.inviteMilestone.description')
})

let disposed = false

function money(value: number): string {
  return Number(value || 0).toFixed(2)
}

function tierPositionPct(index: number): number {
  if (!tiers.value.length) return 0
  if (!denseMilestones.value && maxInvites.value > 0) {
    return (tiers.value[index]!.invites / maxInvites.value) * 100
  }
  return equidistantTierPositionPct(index)
}

function equidistantTierPositionPct(index: number): number {
  if (!tiers.value.length) return 0
  return ((index + 1) / tiers.value.length) * 100
}

function chestClass(tier: InviteMilestoneTierStatus): string {
  if (tier.claimed) return 'ms-chest--claimed'
  if (tier.claimable) return 'ms-chest--ready'
  if (tier.blocked_by_recharge) return 'ms-chest--payment'
  return 'ms-chest--locked'
}

function tierLabel(tier: InviteMilestoneTierStatus): string {
  if (tier.blocked_by_recharge) {
    return t('inviteActivities.inviteMilestone.rechargeRemaining', {
      count: Math.max(0, tier.invites - qualifiedInvited.value),
    })
  }
  return t('inviteActivities.inviteMilestone.rewardLabel', { amount: money(tier.reward) })
}

function tierAction(tier: InviteMilestoneTierStatus): string {
  if (tier.claimed) {
    return t('inviteActivities.inviteMilestone.claimedReward', { amount: money(tier.reward) })
  }
  if (tier.blocked_by_recharge) return t('inviteActivities.inviteMilestone.waitingRecharge')
  if (!tier.reached) return t('inviteActivities.inviteMilestone.notReached')
  return t('inviteActivities.inviteMilestone.claim', { amount: money(tier.reward) })
}

async function publicFlagEnabled(force = false): Promise<boolean> {
  if (force || !appStore.publicSettingsLoaded) {
    const settings = await appStore.fetchPublicSettings(force)
    if (!settings) return false
  }
  return isInviteActivitySettingsEnabled(
    appStore.publicSettingsLoaded,
    appStore.cachedPublicSettings,
    'subnexus_invite_milestone_enabled',
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
    const result = await getInviteMilestoneStatus()
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

async function claimTier(tier: InviteMilestoneTierStatus): Promise<void> {
  if (!tier.claimable || claimingTarget.value !== null || disposed) return
  claimingTarget.value = tier.invites
  showReward.value = false

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
    claimingTarget.value = null
    return
  }

  try {
    const result = await claimInviteMilestone(tier.invites)
    if (disposed) return
    status.value = result
    featureDisabled.value = !result.enabled
    if (result.just_claimed_reward === undefined) return

    rewardAmount.value = result.just_claimed_reward
    showReward.value = true
    appStore.showSuccess(
      t('inviteActivities.inviteMilestone.received', { amount: money(rewardAmount.value) }),
    )
  } catch (error) {
    if (disposed) return
    appStore.showError(extractApiErrorMessage(error, t('inviteActivities.common.claimFailed')))
    await loadStatus()
  } finally {
    if (!disposed) claimingTarget.value = null
  }
}

function goAffiliate(): void {
  void router.push('/affiliate')
}

onMounted(() => {
  void loadStatus()
})
onBeforeUnmount(() => {
  disposed = true
})
</script>

<style scoped>
.card {
  border-color: rgba(229, 231, 235, 0.7);
  border-radius: 0.75rem;
}
html.dark .card { border-color: rgba(51, 65, 85, 0.6); }
.btn { border-radius: 0.5rem; }
.btn-sm { border-radius: 0.5rem; }
.btn-primary {
  background: #0d9488;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
}
.btn-primary:hover { background: #0f766e; box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1); }
html.dark .btn-primary { background: #0d9488; }
html.dark .btn-primary:hover { background: #14b8a6; }

.ms-stage {
  background:
    radial-gradient(circle at 16% 14%, rgba(56, 189, 248, 0.3), transparent 42%),
    radial-gradient(circle at 86% 86%, rgba(139, 92, 246, 0.32), transparent 46%),
    linear-gradient(160deg, #11233f 0%, #1e3a5f 46%, #3b2a6b 100%);
  box-shadow: 0 26px 60px -28px rgba(30, 58, 95, 0.9);
}
.ms-stage__glow { background: radial-gradient(circle at 50% 24%, rgba(255, 255, 255, 0.15), transparent 55%); }
.ms-stage__stars {
  background-image:
    radial-gradient(1.5px 1.5px at 14% 22%, rgba(255, 255, 255, 0.7), transparent),
    radial-gradient(1.5px 1.5px at 72% 16%, rgba(255, 255, 255, 0.5), transparent),
    radial-gradient(1.5px 1.5px at 40% 78%, rgba(255, 255, 255, 0.5), transparent),
    radial-gradient(1.5px 1.5px at 88% 60%, rgba(255, 255, 255, 0.5), transparent);
  animation: ms-twinkle 3.6s ease-in-out infinite;
}
@keyframes ms-twinkle { 0%, 100% { opacity: 0.35; } 50% { opacity: 0.8; } }

.ms-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-radius: 9999px;
  padding: 5px 14px;
  font-size: 13px;
  font-weight: 700;
  color: #4c1d95;
  background: linear-gradient(180deg, #fcd34d, #f59e0b);
  box-shadow: 0 6px 16px -6px rgba(251, 191, 36, 0.9);
  animation: ms-badge-bounce 1.4s ease-in-out infinite;
}
@keyframes ms-badge-bounce { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-3px); } }

.ms-track-scroll {
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
}
.ms-track-space { min-width: 100%; }

@media (max-width: 639px) {
  .ms-fill { width: var(--ms-mobile-width) !important; }
  .ms-node { left: var(--ms-mobile-left) !important; }
}

.ms-track {
  position: absolute;
  left: 0;
  right: 0;
  top: 50%;
  height: 12px;
  transform: translateY(-50%);
  border-radius: 9999px;
  background: rgba(255, 255, 255, 0.14);
  box-shadow: inset 0 1px 3px rgba(0, 0, 0, 0.35);
  overflow: hidden;
}
.ms-fill {
  height: 100%;
  border-radius: 9999px;
  background: linear-gradient(90deg, #34d399, #fbbf24, #fb923c);
  background-size: 200% 100%;
  box-shadow: 0 0 14px rgba(251, 191, 36, 0.6);
  transition: width 0.7s cubic-bezier(0.16, 1, 0.3, 1);
  animation: ms-flow 2.4s linear infinite;
}
@keyframes ms-flow { to { background-position: 200% 0; } }

.ms-node {
  position: absolute;
  top: 0;
  bottom: 0;
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
}
.ms-node__req {
  font-size: 12px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.85);
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.4);
}
.ms-node__reward {
  font-size: 12px;
  font-weight: 700;
  color: rgba(255, 255, 255, 0.55);
}
.ms-node__reward--on { color: #fde68a; }

.ms-chest {
  position: relative;
  display: flex;
  height: 52px;
  width: 52px;
  align-items: center;
  justify-content: center;
  border-radius: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  transition: transform 0.15s ease, box-shadow 0.15s ease, filter 0.15s ease;
}
.ms-chest--locked {
  background: linear-gradient(160deg, #64748b, #475569);
  color: #cbd5e1;
  filter: grayscale(0.4) brightness(0.85);
  cursor: not-allowed;
}
.ms-chest--ready {
  background: linear-gradient(160deg, #fcd34d, #f59e0b);
  color: #7c2d12;
  border-color: #fff7cc;
  cursor: pointer;
  box-shadow: 0 0 0 3px rgba(253, 224, 71, 0.45), 0 10px 22px -6px rgba(251, 191, 36, 0.9);
  animation: ms-chest-bounce 1s ease-in-out infinite;
}
.ms-chest--ready:hover { transform: scale(1.1); }
.ms-chest--payment {
  background: linear-gradient(160deg, #fef3c7, #f59e0b);
  color: #92400e;
  border-color: rgba(253, 230, 138, 0.9);
  cursor: not-allowed;
  box-shadow: 0 8px 18px -10px rgba(245, 158, 11, 0.9);
}
.ms-chest--claimed {
  background: linear-gradient(160deg, #34d399, #059669);
  color: #ffffff;
  border-color: rgba(255, 255, 255, 0.6);
  cursor: default;
}
@keyframes ms-chest-bounce {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-5px); }
}
.ms-chest__spark {
  position: absolute;
  top: -8px;
  right: -8px;
  color: #fff;
  filter: drop-shadow(0 0 4px rgba(253, 224, 71, 1));
  animation: ms-spark 1.2s ease-in-out infinite;
}
@keyframes ms-spark { 0%, 100% { opacity: 0.5; transform: scale(0.8); } 50% { opacity: 1; transform: scale(1.15); } }

.ms-reveal {
  position: absolute;
  inset: 0;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(10, 20, 40, 0.55);
  backdrop-filter: blur(2px);
}
.ms-reveal__card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 28px 38px;
  border-radius: 22px;
  background: linear-gradient(160deg, rgba(255, 255, 255, 0.97), rgba(255, 247, 237, 0.97));
  box-shadow: 0 24px 60px -20px rgba(0, 0, 0, 0.7);
  border: 2px solid rgba(251, 191, 36, 0.6);
  animation: ms-card-pop 0.5s cubic-bezier(0.16, 1, 0.3, 1);
}
.ms-reveal__title { font-size: 14px; font-weight: 700; letter-spacing: 2px; color: #b45309; }
.ms-reveal__amount {
  font-size: 40px;
  font-weight: 900;
  background: linear-gradient(90deg, #f59e0b, #ef4444, #ec4899);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.ms-reveal__hint { font-size: 12px; color: #6b7280; }
@keyframes ms-card-pop { 0% { transform: scale(0.6); opacity: 0; } 100% { transform: scale(1); opacity: 1; } }
.ms-reveal-enter-active { transition: opacity 0.3s ease; }
.ms-reveal-enter-from { opacity: 0; }
</style>
