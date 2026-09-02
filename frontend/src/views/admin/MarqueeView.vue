<template>
  <AppLayout>
    <section class="space-y-6" data-testid="admin-marquee-page">
      <header class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div class="mb-2 flex items-center gap-2 text-primary-600 dark:text-primary-400">
            <Icon name="bell" size="md" aria-hidden="true" />
            <span class="text-xs font-semibold uppercase">{{ t('admin.marquee.title') }}</span>
          </div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.marquee.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.marquee.description') }}</p>
        </div>
        <div class="flex items-center gap-3">
          <span class="text-sm text-gray-600 dark:text-dark-300">{{ t('admin.marquee.enabled') }}</span>
          <Toggle
            v-model="config.enabled"
            :disabled="configLoading || configSaving || configLoadFailed"
            data-testid="marquee-enabled"
            @update:model-value="handleConfigToggle"
          />
        </div>
      </header>

      <div
        v-if="configLoadFailed"
        class="rounded-md border border-red-200 bg-red-50 px-5 py-4 text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
        role="alert"
        data-testid="marquee-config-error"
      >
        <p class="text-sm font-medium">{{ t('admin.marquee.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary btn-sm mt-3 inline-flex items-center gap-2" @click="loadConfig">
          <Icon name="refresh" size="sm" aria-hidden="true" />
          <span>{{ t('admin.marquee.retry') }}</span>
        </button>
      </div>

      <div v-else-if="configLoading" class="flex justify-center py-16" aria-busy="true">
        <Icon name="refresh" size="lg" class="animate-spin text-primary-500" aria-hidden="true" />
      </div>

      <div
        v-else-if="!config.enabled"
        class="border-l-2 border-amber-400 px-4 py-3 text-sm text-gray-600 dark:text-dark-300"
        data-testid="marquee-disabled"
      >
        {{ t('admin.marquee.disabled') }}
      </div>

      <div v-else class="grid gap-8 xl:grid-cols-[minmax(18rem,24rem)_minmax(0,1fr)]">
        <form class="space-y-4" data-testid="marquee-form" @submit.prevent="saveBroadcast">
          <div class="flex items-center justify-between gap-3 border-b border-gray-200 pb-3 dark:border-dark-700">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ editingId === null ? t('admin.marquee.create') : t('admin.marquee.edit') }}
            </h2>
            <button v-if="editingId !== null" type="button" class="btn btn-secondary btn-sm" @click="resetForm">
              {{ t('common.cancel') }}
            </button>
          </div>

          <label class="block space-y-1.5">
            <span class="input-label">{{ t('admin.marquee.titleField') }}</span>
            <input v-model="form.title" class="input" maxlength="120" autocomplete="off" />
          </label>
          <label class="block space-y-1.5">
            <span class="input-label">{{ t('admin.marquee.content') }}</span>
            <textarea v-model="form.content" class="input min-h-28 resize-y" maxlength="4000" required></textarea>
          </label>
          <label class="block space-y-1.5">
            <span class="input-label">{{ t('admin.marquee.priority') }}</span>
            <input v-model.number="form.priority" class="input" type="number" min="0" max="1000" step="1" />
          </label>
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-1">
            <label class="block space-y-1.5">
              <span class="input-label">{{ t('admin.marquee.startsAt') }}</span>
              <input v-model="form.start_at" class="input" type="datetime-local" />
            </label>
            <label class="block space-y-1.5">
              <span class="input-label">{{ t('admin.marquee.endsAt') }}</span>
              <input v-model="form.end_at" class="input" type="datetime-local" />
            </label>
          </div>
          <label class="flex items-center justify-between gap-3 border-y border-gray-100 py-3 text-sm text-gray-700 dark:border-dark-700 dark:text-dark-200">
            <span>{{ t('admin.marquee.published') }}</span>
            <Toggle v-model="form.enabled" />
          </label>
          <button type="submit" class="btn btn-primary inline-flex w-full items-center justify-center gap-2" :disabled="saving || !form.content.trim()">
            <Icon name="check" size="sm" aria-hidden="true" />
            <span>{{ t('common.save') }}</span>
          </button>
        </form>

        <section class="min-w-0">
          <div class="mb-3 flex items-center justify-end">
            <button
              type="button"
              class="btn-icon"
              :title="t('common.refresh')"
              :aria-label="t('common.refresh')"
              :disabled="listLoading"
              @click="loadBroadcasts"
            >
              <Icon name="refresh" size="sm" :class="{ 'animate-spin': listLoading }" aria-hidden="true" />
            </button>
          </div>

          <div v-if="listFailed" class="border-l-2 border-red-500 px-4 py-3 text-sm text-red-700 dark:text-red-300" role="alert">
            {{ t('admin.marquee.listFailed') }}
          </div>
          <div class="overflow-x-auto border-y border-gray-200 dark:border-dark-700">
            <table class="min-w-full">
              <thead>
                <tr>
                  <th class="table-th">{{ t('admin.marquee.content') }}</th>
                  <th class="table-th">{{ t('admin.marquee.schedule') }}</th>
                  <th class="table-th">{{ t('admin.marquee.published') }}</th>
                  <th class="table-th text-right">{{ t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr v-if="listLoading"><td colspan="4" class="px-5 py-12 text-center"><Icon name="refresh" size="md" class="mx-auto animate-spin text-primary-500" /></td></tr>
                <tr v-else-if="broadcasts.length === 0"><td colspan="4" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('admin.marquee.empty') }}</td></tr>
                <tr v-for="item in broadcasts" v-else :key="item.id">
                  <td class="table-td max-w-md">
                    <p v-if="item.title" class="font-medium text-gray-900 dark:text-white">{{ item.title }}</p>
                    <p class="truncate text-sm text-gray-600 dark:text-dark-300">{{ item.content }}</p>
                    <p class="mt-1 text-xs text-gray-400">#{{ item.priority }}</p>
                  </td>
                  <td class="table-td whitespace-nowrap text-xs">{{ scheduleLabel(item) }}</td>
                  <td class="table-td"><span :class="item.enabled ? 'badge badge-success' : 'badge badge-gray'">{{ item.enabled ? t('common.enabled') : t('common.disabled') }}</span></td>
                  <td class="table-td text-right whitespace-nowrap">
                    <button type="button" class="btn-icon mr-1" :title="t('common.edit')" :aria-label="t('common.edit')" @click="editBroadcast(item)"><Icon name="edit" size="sm" /></button>
                    <button type="button" class="btn-icon text-red-500" :title="t('common.delete')" :aria-label="t('common.delete')" @click="requestDelete(item)"><Icon name="trash" size="sm" /></button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </section>

    <ConfirmDialog
      :show="deleting !== null"
      :title="t('admin.marquee.deleteTitle')"
      :message="t('admin.marquee.deleteConfirm', { title: deleting?.title || deleting?.content || '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="deleting = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  createMarqueeBroadcast,
  deleteMarqueeBroadcast,
  getMarqueeConfig,
  listAdminMarqueeBroadcasts,
  updateMarqueeBroadcast,
  updateMarqueeConfig,
  type MarqueeBroadcast,
  type MarqueeBroadcastInput,
} from '@/api/marquee'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const config = reactive({ enabled: false })
const configLoading = ref(true)
const configSaving = ref(false)
const configLoadFailed = ref(false)
const confirmedEnabled = ref(false)
const broadcasts = ref<MarqueeBroadcast[]>([])
const listLoading = ref(false)
const listFailed = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const deleting = ref<MarqueeBroadcast | null>(null)
const form = reactive({ title: '', content: '', enabled: true, priority: 0, start_at: '', end_at: '' })

function syncPublicFlag(enabled: boolean): void {
  if (appStore.cachedPublicSettings) {
    appStore.cachedPublicSettings = { ...appStore.cachedPublicSettings, subnexus_marquee_enabled: enabled }
  }
}

async function loadConfig(): Promise<void> {
  configLoading.value = true
  configLoadFailed.value = false
  broadcasts.value = []
  try {
    const result = await getMarqueeConfig()
    const enabled = result?.enabled === true
    config.enabled = enabled
    confirmedEnabled.value = enabled
    syncPublicFlag(enabled)
    if (enabled) await loadBroadcasts()
  } catch (error) {
    config.enabled = false
    confirmedEnabled.value = false
    configLoadFailed.value = true
    syncPublicFlag(false)
    appStore.showError(extractApiErrorMessage(error, t('admin.marquee.loadFailed')))
  } finally {
    configLoading.value = false
  }
}

async function handleConfigToggle(enabled: boolean): Promise<void> {
  if (configSaving.value) return
  configSaving.value = true
  const previous = confirmedEnabled.value
  try {
    const result = await updateMarqueeConfig({ enabled: enabled === true })
    const saved = result?.enabled === true
    config.enabled = saved
    confirmedEnabled.value = saved
    syncPublicFlag(saved)
    if (saved) await loadBroadcasts()
    else {
      broadcasts.value = []
      resetForm()
    }
    appStore.showSuccess(t('admin.marquee.configSaved'))
  } catch (error) {
    config.enabled = previous
    appStore.showError(extractApiErrorMessage(error, t('admin.marquee.configSaveFailed')))
  } finally {
    configSaving.value = false
  }
}

async function loadBroadcasts(): Promise<void> {
  if (!confirmedEnabled.value && !config.enabled) return
  listLoading.value = true
  listFailed.value = false
  try {
    broadcasts.value = await listAdminMarqueeBroadcasts()
  } catch (error) {
    broadcasts.value = []
    listFailed.value = true
    appStore.showError(extractApiErrorMessage(error, t('admin.marquee.listFailed')))
  } finally {
    listLoading.value = false
  }
}

function toAPITime(value: string): string | null {
  if (!value) return null
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? null : parsed.toISOString()
}

function toLocalInput(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 16)
}

function buildInput(): MarqueeBroadcastInput | null {
  const content = form.content.trim()
  const priority = Math.trunc(Number(form.priority))
  if (!content || content.length > 4000 || !Number.isFinite(priority) || priority < 0 || priority > 1000) return null
  const startAt = toAPITime(form.start_at)
  const endAt = toAPITime(form.end_at)
  if (startAt && endAt && new Date(endAt) < new Date(startAt)) return null
  return {
    title: form.title.trim(),
    content,
    enabled: form.enabled === true,
    priority,
    start_at: startAt,
    end_at: endAt,
  }
}

async function saveBroadcast(): Promise<void> {
  if (saving.value || !confirmedEnabled.value) return
  const input = buildInput()
  if (!input) {
    appStore.showError(t('admin.marquee.saveFailed'))
    return
  }
  saving.value = true
  try {
    if (editingId.value === null) await createMarqueeBroadcast(input)
    else await updateMarqueeBroadcast(editingId.value, input)
    resetForm()
    await loadBroadcasts()
    appStore.showSuccess(t('admin.marquee.saved'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.marquee.saveFailed')))
  } finally {
    saving.value = false
  }
}

function editBroadcast(item: MarqueeBroadcast): void {
  editingId.value = item.id
  form.title = item.title
  form.content = item.content
  form.enabled = item.enabled
  form.priority = item.priority
  form.start_at = toLocalInput(item.start_at)
  form.end_at = toLocalInput(item.end_at)
}

function resetForm(): void {
  editingId.value = null
  Object.assign(form, { title: '', content: '', enabled: true, priority: 0, start_at: '', end_at: '' })
}

function requestDelete(item: MarqueeBroadcast): void {
  deleting.value = item
}

async function confirmDelete(): Promise<void> {
  if (!deleting.value || !confirmedEnabled.value) return
  const id = deleting.value.id
  try {
    await deleteMarqueeBroadcast(id)
    deleting.value = null
    if (editingId.value === id) resetForm()
    await loadBroadcasts()
    appStore.showSuccess(t('admin.marquee.deleted'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.marquee.deleteFailed')))
  }
}

function scheduleLabel(item: MarqueeBroadcast): string {
  if (!item.start_at && !item.end_at) return t('admin.marquee.noSchedule')
  const format = (value?: string | null) => value
    ? new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value))
    : '…'
  return `${format(item.start_at)} - ${format(item.end_at)}`
}

onMounted(() => { void loadConfig() })
</script>
