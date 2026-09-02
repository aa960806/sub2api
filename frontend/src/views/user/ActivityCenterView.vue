<template>
  <AppLayout>
    <section class="activity-page space-y-5" data-testid="activity-center-page">
      <header class="flex flex-col gap-3 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div class="mb-2 flex items-center gap-2 text-primary-600 dark:text-primary-400">
            <Icon name="gift" size="md" aria-hidden="true" />
            <span class="text-xs font-semibold uppercase">{{ t('nav.activities') }}</span>
          </div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('activityCenter.description') }}</p>
        </div>
        <button
          type="button"
          class="btn btn-secondary inline-flex items-center gap-2 self-start sm:self-auto"
          :disabled="loading"
          :title="t('common.refresh')"
          @click="loadData"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
          <span>{{ t('common.refresh') }}</span>
        </button>
      </header>

      <div v-if="loading" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3" aria-busy="true">
        <div v-for="index in 3" :key="index" class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <div class="h-2 bg-gray-200 dark:bg-dark-700"></div>
          <div class="space-y-3 p-5">
            <div class="h-5 w-2/3 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
            <div class="h-4 w-1/2 animate-pulse rounded bg-gray-100 dark:bg-dark-700/80"></div>
            <div class="h-12 animate-pulse rounded bg-gray-100 dark:bg-dark-700/80"></div>
            <div class="h-9 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></div>
          </div>
        </div>
      </div>

      <div v-else-if="!enabled" class="rounded-lg border border-dashed border-gray-300 bg-white px-6 py-14 text-center dark:border-dark-600 dark:bg-dark-800" data-testid="activity-center-disabled">
        <Icon name="gift" size="xl" class="mx-auto text-gray-300 dark:text-dark-600" aria-hidden="true" />
        <p class="mt-4 text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('activityCenter.disabledTitle') }}</p>
      </div>

      <div v-else-if="items.length === 0" class="rounded-lg border border-gray-200 bg-white px-6 py-14 text-center dark:border-dark-700 dark:bg-dark-800" data-testid="activity-center-empty">
        <Icon name="gift" size="xl" class="mx-auto text-gray-300 dark:text-dark-600" aria-hidden="true" />
        <p class="mt-4 text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('activityCenter.emptyTitle') }}</p>
      </div>

      <div v-else class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3" data-testid="activity-center-items">
        <article
          v-for="(item, index) in items"
          :key="item.id"
          class="flex min-h-[15rem] flex-col overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm transition-shadow hover:shadow-md dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="relative h-28 overflow-hidden border-b border-gray-100 dark:border-dark-700" :class="accentClass(index)">
            <img
              v-if="safeCoverUrl(item.cover_url)"
              :src="safeCoverUrl(item.cover_url)"
              :alt="item.title"
              class="h-full w-full object-cover"
              loading="lazy"
              referrerpolicy="no-referrer"
            />
            <div v-else class="flex h-full items-center justify-center text-white/80">
              <Icon :name="resolveIcon(item.icon)" size="xl" aria-hidden="true" />
            </div>
            <span class="absolute right-3 top-3 rounded-md bg-black/45 px-2 py-1 text-xs font-medium text-white">
              {{ t('activityCenter.activeLabel') }}
            </span>
          </div>

          <div class="flex flex-1 flex-col p-5">
            <div class="flex items-start gap-3">
              <span class="-mt-9 flex h-10 w-10 shrink-0 items-center justify-center rounded-md border-2 border-white bg-gray-800 text-white shadow-sm dark:border-dark-800" :class="accentClass(index)">
                <Icon :name="resolveIcon(item.icon)" size="sm" aria-hidden="true" />
              </span>
              <div class="min-w-0 flex-1">
                <h2 class="break-words text-base font-semibold text-gray-900 dark:text-white">{{ item.title }}</h2>
                <p v-if="item.subtitle" class="mt-1 break-words text-sm font-medium text-primary-600 dark:text-primary-400">{{ item.subtitle }}</p>
              </div>
            </div>
            <p v-if="item.description" class="mt-4 line-clamp-3 break-words text-sm leading-6 text-gray-600 dark:text-dark-300">{{ item.description }}</p>
            <div v-if="item.start_at || item.end_at" class="mt-3 text-xs text-gray-500 dark:text-dark-400">
              <span v-if="item.start_at">{{ t('activityCenter.startsAt', { date: formatActivityDate(item.start_at) }) }}</span>
              <span v-if="item.start_at && item.end_at" aria-hidden="true"> · </span>
              <span v-if="item.end_at">{{ t('activityCenter.endsAt', { date: formatActivityDate(item.end_at) }) }}</span>
            </div>
            <div class="mt-auto pt-5">
              <button
                v-if="hasSafeTarget(item)"
                type="button"
                class="btn btn-primary inline-flex w-full items-center justify-center gap-2 rounded-md"
                @click="openActivity(item)"
              >
                <span>{{ item.action_label || t('activityCenter.openAction') }}</span>
                <Icon :name="item.external_url ? 'externalLink' : 'arrowRight'" size="sm" aria-hidden="true" />
              </button>
              <span v-else class="block text-center text-xs text-gray-400 dark:text-dark-500">{{ t('activityCenter.noTarget') }}</span>
            </div>
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

const activityIconNames = ['gift', 'sparkles', 'trophy', 'badge', 'bolt', 'calendar', 'link', 'fire'] as const
type ActivityIconName = (typeof activityIconNames)[number]

function resolveIcon(value: string | undefined): ActivityIconName {
  return activityIconNames.includes(value as ActivityIconName)
    ? (value as ActivityIconName)
    : 'gift'
}

function safeCoverUrl(value: string | undefined): string {
	return isSafeActivityExternalUrl(value) ? sanitizeUrl(value) : ''
}

function hasSafeTarget(item: ActivityCenterItem): boolean {
  if (item.external_url) return isSafeActivityExternalUrl(item.external_url)
  return isSafeActivityRoutePath(item.route_path)
}

function formatActivityDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

function accentClass(index: number): string {
  const classes = [
    'bg-teal-600 dark:bg-teal-700',
    'bg-sky-600 dark:bg-sky-700',
    'bg-emerald-600 dark:bg-emerald-700',
    'bg-amber-600 dark:bg-amber-700',
    'bg-rose-600 dark:bg-rose-700',
  ]
  return classes[index % classes.length]
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
