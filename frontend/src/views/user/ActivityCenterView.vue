<template>
  <AppLayout legacy-surface>
    <section class="space-y-6" data-testid="activity-center-page">
      <header class="ac-header relative overflow-hidden rounded-2xl px-6 py-6 sm:px-8 sm:py-7">
        <div class="ac-header__glow pointer-events-none absolute inset-0"></div>
        <div class="relative flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex items-center gap-3">
            <span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-white/15 text-white ring-1 ring-white/25">
              <Icon name="sparkles" size="md" aria-hidden="true" />
            </span>
            <div>
              <h1 class="text-xl font-bold text-white sm:text-2xl">{{ t('activityCenter.title') }}</h1>
              <p class="mt-0.5 text-sm text-white/75">
                {{ enabled && items.length
                  ? t('activityCenter.availableCount', { count: items.length })
                  : t('activityCenter.summary') }}
              </p>
            </div>
          </div>
          <button
            type="button"
            class="ac-refresh"
            :disabled="loading"
            :title="t('common.refresh')"
            @click="loadData"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
            <span class="ml-1.5 text-sm font-medium">{{ t('common.refresh') }}</span>
          </button>
        </div>
      </header>

      <div v-if="loading" class="grid gap-5 md:grid-cols-2 xl:grid-cols-3" aria-busy="true">
        <div v-for="index in 3" :key="'skeleton-' + index" class="card overflow-hidden p-0">
          <div class="ac-skeleton h-28 w-full rounded-none"></div>
          <div class="space-y-3 p-5">
            <div class="ac-skeleton h-4 w-2/3"></div>
            <div class="ac-skeleton h-3 w-1/2"></div>
            <div class="ac-skeleton h-3 w-full"></div>
            <div class="ac-skeleton mt-3 h-9 w-full"></div>
          </div>
        </div>
      </div>

      <div
        v-else-if="!enabled"
        class="card flex flex-col items-center justify-center px-5 py-16 text-center"
        data-testid="activity-center-disabled"
      >
        <span class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gray-100 text-gray-400 dark:bg-dark-800 dark:text-dark-500">
          <Icon name="gift" size="xl" aria-hidden="true" />
        </span>
        <p class="text-sm font-medium text-gray-600 dark:text-dark-300">{{ t('activityCenter.disabledTitle') }}</p>
        <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">{{ t('activityCenter.disabledSubtitle') }}</p>
      </div>

      <div
        v-else-if="items.length === 0"
        class="card flex flex-col items-center justify-center px-5 py-16 text-center"
        data-testid="activity-center-empty"
      >
        <span class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gray-100 text-gray-400 dark:bg-dark-800 dark:text-dark-500">
          <Icon name="gift" size="xl" aria-hidden="true" />
        </span>
        <p class="text-sm font-medium text-gray-600 dark:text-dark-300">{{ t('activityCenter.emptyTitle') }}</p>
        <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">{{ t('activityCenter.emptySubtitle') }}</p>
      </div>

      <div v-else class="grid gap-5 md:grid-cols-2 xl:grid-cols-3" data-testid="activity-center-items">
        <article
          v-for="(item, index) in items"
          :key="item.id"
          class="ac-card group card overflow-hidden p-0"
          :style="{ animationDelay: Math.min(index, 8) * 60 + 'ms' }"
        >
          <div class="ac-banner" :style="bannerStyle(index, item)">
            <img
              v-if="safeCoverUrl(item.cover_url)"
              :src="safeCoverUrl(item.cover_url)"
              :alt="item.title"
              class="ac-cover"
              loading="lazy"
              referrerpolicy="no-referrer"
            />
            <span v-else class="ac-banner__deco">
              <Icon name="gift" size="xl" aria-hidden="true" />
            </span>
            <span class="ac-banner__shade"></span>
            <span class="ac-live">
              <span class="ac-live__dot" aria-hidden="true"></span>
              {{ t('activityCenter.activeLabel') }}
            </span>
          </div>

          <span class="ac-badge" :style="badgeStyle(index)">
            <Icon name="gift" size="md" aria-hidden="true" />
          </span>

          <div class="ac-body">
            <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ item.title }}</h2>
            <p v-if="item.subtitle" class="mt-1 truncate text-sm font-medium" :style="{ color: accent(index)[0] }">
              {{ item.subtitle }}
            </p>
            <p v-if="item.description" class="ac-description mt-3 text-sm text-gray-600 dark:text-dark-300">
              {{ item.description }}
            </p>
            <button
              v-if="hasSafeTarget(item)"
              type="button"
              class="ac-cta"
              :style="ctaStyle(index)"
              @click="openActivity(item)"
            >
              <span>{{ item.action_label || t('activityCenter.openAction') }}</span>
              <Icon name="arrowRight" size="sm" aria-hidden="true" />
            </button>
            <span v-else class="ac-no-target">{{ t('activityCenter.noTarget') }}</span>
          </div>
        </article>
      </div>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { listActivityCenter, type ActivityCenterItem } from '@/api/activityCenter'
import { useAppStore } from '@/stores/app'
import { sanitizeUrl } from '@/utils/url'
import {
  isActivityCenterSettingsEnabled,
  isSafeActivityExternalUrl,
  isSafeActivityRoutePath,
} from '@/utils/activityCenter'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const loading = ref(false)
const enabled = ref(false)
const items = ref<ActivityCenterItem[]>([])

function safeCoverUrl(value: string | undefined): string {
  return isSafeActivityExternalUrl(value) ? sanitizeUrl(value) : ''
}

function hasSafeTarget(item: ActivityCenterItem): boolean {
  if (item.external_url) return isSafeActivityExternalUrl(item.external_url)
  return isSafeActivityRoutePath(item.route_path)
}

const palette: [string, string][] = [
  ['#6366f1', '#8b5cf6'],
  ['#ec4899', '#f43f5e'],
  ['#f59e0b', '#f97316'],
  ['#10b981', '#14b8a6'],
  ['#06b6d4', '#3b82f6'],
  ['#8b5cf6', '#d946ef'],
]

function accent(index: number): [string, string] {
  return palette[index % palette.length]
}

function bannerStyle(index: number, item: ActivityCenterItem): Record<string, string> {
  if (safeCoverUrl(item.cover_url)) return {}
  const [start, end] = accent(index)
  return { background: 'linear-gradient(135deg, ' + start + ', ' + end + ')' }
}

function badgeStyle(index: number): Record<string, string> {
  const [start, end] = accent(index)
  return { background: 'linear-gradient(135deg, ' + start + ', ' + end + ')' }
}

function ctaStyle(index: number): Record<string, string> {
  const [start, end] = accent(index)
  return { backgroundImage: 'linear-gradient(135deg, ' + start + ', ' + end + ')' }
}

async function loadData(): Promise<void> {
  loading.value = true
  try {
    // Deep links and direct component mounts can bypass the router guard. Do
    // not query activity rows unless public settings were loaded successfully
    // and the independent migration switch is explicitly enabled.
    if (!appStore.publicSettingsLoaded) await appStore.fetchPublicSettings()
    if (!isActivityCenterSettingsEnabled(appStore.publicSettingsLoaded, appStore.cachedPublicSettings)) {
      enabled.value = false
      items.value = []
      return
    }
    const result = await listActivityCenter()
    // Defense in depth: only an explicit enabled response and custom rows are
    // rendered, even if an older/misconfigured server returns other types.
    enabled.value = result?.enabled === true
    items.value = enabled.value
      ? (result.items ?? []).filter((item) => item.activity_type === 'custom')
      : []
  } catch (error) {
    enabled.value = false
    items.value = []
    const message = error instanceof Error ? error.message : undefined
    appStore.showError(message || t('activityCenter.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openActivity(item: ActivityCenterItem): void {
  if (item.external_url) {
    if (!isSafeActivityExternalUrl(item.external_url)) {
      appStore.showError(t('activityCenter.invalidTarget'))
      return
    }
    const url = sanitizeUrl(item.external_url)
    window.open(url, '_blank', 'noopener,noreferrer')
    return
  }

  if (isSafeActivityRoutePath(item.route_path)) {
    void router.push(item.route_path)
    return
  }

  appStore.showError(t('activityCenter.invalidTarget'))
}

onMounted(() => {
  void loadData()
})
</script>

<style scoped>
.card {
  border-color: rgb(229 231 235 / 70%);
}
.card:not(.ac-card) { border-radius: 0.75rem; }
html.dark .card { border-color: rgb(51 65 85 / 60%); }
.ac-header {
  background: linear-gradient(120deg, #4f46e5 0%, #7c3aed 50%, #c026d3 100%);
  box-shadow: 0 18px 40px -24px rgba(124, 58, 237, 0.8);
}

.ac-header__glow {
  background:
    radial-gradient(circle at 12% 20%, rgba(255, 255, 255, 0.22), transparent 45%),
    radial-gradient(circle at 90% 90%, rgba(255, 255, 255, 0.12), transparent 50%);
}

.ac-refresh {
  display: inline-flex;
  align-items: center;
  border: 1px solid rgba(255, 255, 255, 0.28);
  border-radius: 9999px;
  padding: 7px 16px;
  color: #fff;
  background: rgba(255, 255, 255, 0.16);
  transition: background 0.2s ease, transform 0.2s ease;
}

.ac-refresh:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.26);
}

.ac-refresh:disabled {
  cursor: not-allowed;
  opacity: 0.7;
}

.ac-card {
  position: relative;
  display: flex;
  flex-direction: column;
  border-radius: 18px;
  transition: transform 0.25s ease, box-shadow 0.25s ease;
  animation: ac-in 0.5s cubic-bezier(0.16, 1, 0.3, 1) both;
}

.ac-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 22px 40px -22px rgba(31, 41, 55, 0.45);
}

@keyframes ac-in {
  from {
    opacity: 0;
    transform: translateY(14px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.ac-banner {
  position: relative;
  height: 112px;
  overflow: hidden;
}

.ac-cover {
  height: 100%;
  width: 100%;
  object-fit: cover;
  transition: transform 0.4s ease;
}

.ac-card:hover .ac-cover {
  transform: scale(1.06);
}

.ac-banner__deco {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(255, 255, 255, 0.32);
  transform: scale(2.4) rotate(-8deg);
}

.ac-banner__shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, transparent 40%, rgba(0, 0, 0, 0.18) 100%);
}

.ac-live {
  position: absolute;
  top: 12px;
  right: 12px;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border-radius: 9999px;
  padding: 3px 10px;
  font-size: 11px;
  font-weight: 600;
  color: #fff;
  background: rgba(0, 0, 0, 0.28);
  backdrop-filter: blur(4px);
}

.ac-live__dot {
  height: 6px;
  width: 6px;
  border-radius: 9999px;
  background: #34d399;
  box-shadow: 0 0 0 0 rgba(52, 211, 153, 0.7);
  animation: ac-pulse 1.6s ease-in-out infinite;
}

@keyframes ac-pulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(52, 211, 153, 0.6);
  }

  50% {
    box-shadow: 0 0 0 5px rgba(52, 211, 153, 0);
  }
}

.ac-badge {
  position: absolute;
  top: 88px;
  left: 18px;
  z-index: 2;
  display: flex;
  height: 48px;
  width: 48px;
  align-items: center;
  justify-content: center;
  border-radius: 14px;
  color: #fff;
  box-shadow: 0 8px 18px -6px rgba(0, 0, 0, 0.4);
}

html.dark .ac-badge {
  box-shadow: 0 8px 18px -6px rgba(0, 0, 0, 0.6), 0 0 0 3px rgba(17, 24, 39, 0.6);
}

.ac-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  padding: 28px 18px 18px;
}

.ac-description {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
}

.ac-cta,
.ac-no-target {
  margin-top: auto;
  border-radius: 12px;
  padding: 10px 16px;
  font-size: 14px;
  font-weight: 600;
}

.ac-cta {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: #fff;
  box-shadow: 0 8px 18px -8px rgba(79, 70, 229, 0.7);
  transition: transform 0.15s ease, filter 0.15s ease, box-shadow 0.15s ease;
}

.ac-cta:hover {
  filter: brightness(1.06);
  transform: translateY(-1px);
}

.ac-cta:active {
  transform: translateY(0);
}

.ac-no-target {
  display: block;
  text-align: center;
  color: rgb(156 163 175);
}

html.dark .ac-no-target {
  color: rgb(75 85 99);
}

.ac-skeleton {
  border-radius: 8px;
  background: linear-gradient(
    90deg,
    rgba(148, 163, 184, 0.18) 25%,
    rgba(148, 163, 184, 0.32) 37%,
    rgba(148, 163, 184, 0.18) 63%
  );
  background-size: 400% 100%;
  animation: ac-shimmer 1.4s ease infinite;
}

@keyframes ac-shimmer {
  0% {
    background-position: 100% 0;
  }

  100% {
    background-position: 0 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .ac-card,
  .ac-cover,
  .ac-live__dot,
  .ac-skeleton {
    animation: none;
    transition: none;
  }
}
</style>
