<template>
  <AppLayout>
    <section class="space-y-5" data-testid="admin-activity-center-page">
      <header class="flex flex-col gap-3 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div class="mb-2 flex items-center gap-2 text-primary-600 dark:text-primary-400">
            <Icon name="gift" size="md" aria-hidden="true" />
            <span class="text-xs font-semibold uppercase">{{ t('nav.activityEntries') }}</span>
          </div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.activityCenter.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.description') }}</p>
        </div>
        <div class="flex items-center gap-3">
          <span class="text-sm text-gray-600 dark:text-dark-300">{{ t('admin.activityCenter.enabledLabel') }}</span>
          <Toggle
            v-model="config.enabled"
            :disabled="configLoading || configSaving"
            data-testid="activity-center-enabled"
            @update:model-value="handleConfigToggle"
          />
        </div>
      </header>

      <div v-if="!config.enabled" class="rounded-lg border border-amber-200 bg-amber-50 px-5 py-4 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200" data-testid="activity-center-closed-notice">
        {{ t('admin.activityCenter.disabledHint') }}
      </div>

      <div class="grid gap-5 xl:grid-cols-[minmax(18rem,24rem)_minmax(0,1fr)]">
        <form
          class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800"
          :class="{ 'opacity-60': !config.enabled }"
          @submit.prevent="saveItem"
        >
          <div class="mb-5 flex items-center justify-between gap-3">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ editingId === null ? t('admin.activityCenter.createTitle') : t('admin.activityCenter.editTitle') }}
            </h2>
            <button v-if="editingId !== null" type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="resetForm">
              {{ t('common.cancel') }}
            </button>
          </div>

          <fieldset :disabled="!config.enabled || saving" class="space-y-4">
            <div>
              <label for="activity-slug" class="input-label">{{ t('admin.activityCenter.fields.slug') }}</label>
              <input id="activity-slug" v-model.trim="form.slug" class="input" required maxlength="80" autocomplete="off" :placeholder="t('admin.activityCenter.fields.slugPlaceholder')" />
            </div>

            <div>
              <label for="activity-title" class="input-label">{{ t('admin.activityCenter.fields.title') }}</label>
              <input id="activity-title" v-model.trim="form.title" class="input" required maxlength="120" />
            </div>

            <div>
              <label for="activity-subtitle" class="input-label">{{ t('admin.activityCenter.fields.subtitle') }}</label>
              <input id="activity-subtitle" v-model.trim="form.subtitle" class="input" maxlength="240" />
            </div>

            <div>
              <label for="activity-description" class="input-label">{{ t('admin.activityCenter.fields.description') }}</label>
              <textarea id="activity-description" v-model.trim="form.description" class="input min-h-24 resize-y" maxlength="2000"></textarea>
            </div>

            <div class="grid grid-cols-2 gap-3">
              <div>
                <label for="activity-icon" class="input-label">{{ t('admin.activityCenter.fields.icon') }}</label>
                <input id="activity-icon" v-model.trim="form.icon" class="input" maxlength="64" :placeholder="t('admin.activityCenter.fields.iconPlaceholder')" />
              </div>
              <div>
                <label for="activity-order" class="input-label">{{ t('admin.activityCenter.fields.sortOrder') }}</label>
                <input id="activity-order" v-model.number="form.sort_order" class="input" type="number" step="1" />
              </div>
            </div>

            <div>
              <label for="activity-route" class="input-label">{{ t('admin.activityCenter.fields.routePath') }}</label>
              <input id="activity-route" v-model.trim="form.route_path" class="input" maxlength="255" :placeholder="t('admin.activityCenter.fields.routePlaceholder')" />
            </div>

            <div>
              <label for="activity-external" class="input-label">{{ t('admin.activityCenter.fields.externalUrl') }}</label>
              <input id="activity-external" v-model.trim="form.external_url" class="input" type="url" :placeholder="t('admin.activityCenter.fields.externalPlaceholder')" />
              <p class="input-hint">{{ t('admin.activityCenter.fields.targetHint') }}</p>
            </div>

            <div>
              <label for="activity-cover" class="input-label">{{ t('admin.activityCenter.fields.coverUrl') }}</label>
              <input id="activity-cover" v-model.trim="form.cover_url" class="input" type="url" :placeholder="t('admin.activityCenter.fields.coverPlaceholder')" />
            </div>

            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label for="activity-start" class="input-label">{{ t('admin.activityCenter.fields.startAt') }}</label>
                <input id="activity-start" v-model="form.start_at" class="input" type="datetime-local" />
              </div>
              <div>
                <label for="activity-end" class="input-label">{{ t('admin.activityCenter.fields.endAt') }}</label>
                <input id="activity-end" v-model="form.end_at" class="input" type="datetime-local" />
              </div>
            </div>

            <div>
              <label for="activity-action" class="input-label">{{ t('admin.activityCenter.fields.actionLabel') }}</label>
              <input id="activity-action" v-model.trim="form.action_label" class="input" maxlength="40" />
            </div>

            <div>
              <label for="activity-metadata" class="input-label">{{ t('admin.activityCenter.fields.metadata') }}</label>
              <textarea id="activity-metadata" v-model="metadataDraft" class="input min-h-20 resize-y font-mono text-xs" :placeholder="'{ }'" spellcheck="false"></textarea>
            </div>

            <label class="flex items-center justify-between gap-3 text-sm text-gray-700 dark:text-dark-200">
              <span>{{ t('admin.activityCenter.fields.published') }}</span>
              <Toggle v-model="form.enabled" />
            </label>

            <button
              type="submit"
              class="btn btn-primary w-full"
              data-testid="activity-center-save"
              :disabled="!canSaveItem"
            >
              <Icon v-if="saving" name="refresh" size="sm" class="mr-1.5 animate-spin" aria-hidden="true" />
              {{ editingId === null ? t('admin.activityCenter.createAction') : t('admin.activityCenter.saveAction') }}
            </button>
          </fieldset>
        </form>

        <section class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" data-testid="activity-center-items-panel">
          <div class="flex items-center justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.activityCenter.listTitle') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.customOnly') }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || !config.enabled" :title="t('common.refresh')" @click="loadItems">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
            </button>
          </div>

          <div v-if="!config.enabled" class="px-5 py-14 text-center text-sm text-gray-500 dark:text-dark-400" data-testid="activity-center-items-disabled">
            {{ t('admin.activityCenter.itemsDisabled') }}
          </div>
          <div v-else-if="loading" class="px-5 py-14 text-center text-sm text-gray-500 dark:text-dark-400" aria-busy="true">
            {{ t('common.loading') }}
          </div>
          <div v-else-if="items.length === 0" class="px-5 py-14 text-center text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.activityCenter.empty') }}
          </div>
          <div v-else class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/80">
                <tr>
                  <th class="px-5 py-3 text-left text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.columns.activity') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.columns.target') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.columns.status') }}</th>
                  <th class="px-5 py-3 text-right text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.columns.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="item in items" :key="item.id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60">
                  <td class="max-w-[18rem] px-5 py-4 align-top">
                    <p class="break-words text-sm font-medium text-gray-900 dark:text-white">{{ item.title }}</p>
                    <p class="mt-1 break-all text-xs text-gray-500 dark:text-dark-400">{{ item.slug }}</p>
                  </td>
                  <td class="max-w-[18rem] px-5 py-4 align-top text-sm text-gray-600 dark:text-dark-300">
                    <span class="break-all">{{ item.route_path || item.external_url || t('admin.activityCenter.noTarget') }}</span>
                  </td>
                  <td class="px-5 py-4 align-top">
                    <span :class="item.enabled ? 'badge badge-success' : 'badge badge-gray'">
                      {{ item.enabled ? t('admin.activityCenter.published') : t('admin.activityCenter.unpublished') }}
                    </span>
                  </td>
                  <td class="px-5 py-4 align-top text-right">
                    <div class="flex justify-end gap-1">
                      <button type="button" class="btn-icon" :disabled="!config.enabled || saving" :title="t('common.edit')" @click="editItem(item)">
                        <Icon name="edit" size="sm" aria-hidden="true" />
                      </button>
                      <button type="button" class="btn-icon text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/30" :disabled="!config.enabled || saving" :title="t('common.delete')" @click="askDelete(item)">
                        <Icon name="trash" size="sm" aria-hidden="true" />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </section>

    <ConfirmDialog
      :show="deleteDialog.show"
      :title="t('admin.activityCenter.deleteTitle')"
      :message="t('admin.activityCenter.deleteMessage', { title: deleteDialog.title })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="deleteDialog.show = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores/app'
import {
  createActivityCenterItem,
  deleteActivityCenterItem,
  getActivityCenterConfig,
  listAdminActivityCenterItems,
  updateActivityCenterConfig,
  updateActivityCenterItem,
  type ActivityCenterItem,
  type ActivityCenterItemInput,
} from '@/api/activityCenter'

const { t } = useI18n()
const appStore = useAppStore()

const config = reactive({ enabled: false })
const configLoading = ref(true)
const configSaving = ref(false)
const loading = ref(false)
const saving = ref(false)
const items = ref<ActivityCenterItem[]>([])
const editingId = ref<number | null>(null)
const metadataDraft = ref('{}')

interface ActivityForm {
  slug: string
  title: string
  subtitle: string
  description: string
  icon: string
  cover_url: string
  route_path: string
  external_url: string
  action_label: string
  enabled: boolean
  sort_order: number
  start_at: string
  end_at: string
}

const emptyForm = (): ActivityForm => ({
  slug: '',
  title: '',
  subtitle: '',
  description: '',
  icon: 'gift',
  cover_url: '',
  route_path: '',
  external_url: '',
  action_label: '',
  enabled: true,
  sort_order: 0,
  start_at: '',
  end_at: '',
})

const form = reactive<ActivityForm>(emptyForm())
const deleteDialog = reactive({ show: false, id: 0, title: '' })

const canSaveItem = computed(() => (
  config.enabled &&
  !saving.value &&
  form.slug.trim().length > 0 &&
  form.title.trim().length > 0 &&
  !(form.route_path.trim() && form.external_url.trim())
))

function syncPublicActivityFlag(enabled: boolean): void {
  const current = appStore.cachedPublicSettings
  if (current) {
    appStore.cachedPublicSettings = { ...current, subnexus_activity_center_enabled: enabled }
  }
  try {
    window.dispatchEvent(new CustomEvent('activity-center-config-changed'))
  } catch {
    // Ignore event errors in non-browser test environments.
  }
}

async function loadConfig(): Promise<boolean> {
  configLoading.value = true
  try {
    const result = await getActivityCenterConfig()
    config.enabled = result?.enabled === true
    syncPublicActivityFlag(config.enabled)
    return config.enabled
  } catch (error) {
    config.enabled = false
    items.value = []
    const message = error instanceof Error ? error.message : undefined
    appStore.showError(message || t('admin.activityCenter.loadFailed'))
    return false
  } finally {
    configLoading.value = false
  }
}

async function loadItems(): Promise<void> {
  if (!config.enabled) {
    items.value = []
    return
  }
  loading.value = true
  try {
    const result = await listAdminActivityCenterItems()
    // The first slice is custom-only even if an older endpoint returns rows
    // from a broader activity registry.
    items.value = (result ?? []).filter((item) => item.activity_type === 'custom')
  } catch (error) {
    items.value = []
    const message = error instanceof Error ? error.message : undefined
    appStore.showError(message || t('admin.activityCenter.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function loadData(): Promise<void> {
  const enabled = await loadConfig()
  if (enabled) await loadItems()
}

async function handleConfigToggle(nextValue: boolean): Promise<void> {
  if (configSaving.value) return
  const previous = !nextValue
  configSaving.value = true
  try {
    const result = await updateActivityCenterConfig({ enabled: nextValue })
    config.enabled = result?.enabled === true
    syncPublicActivityFlag(config.enabled)
    if (config.enabled) {
      await loadItems()
    } else {
      items.value = []
      resetForm()
    }
    appStore.showSuccess(t('admin.activityCenter.configSaved'))
  } catch (error) {
    config.enabled = previous
    syncPublicActivityFlag(previous)
    const message = error instanceof Error ? error.message : undefined
    appStore.showError(message || t('admin.activityCenter.configSaveFailed'))
  } finally {
    configSaving.value = false
  }
}

function toApiDateTime(value: string): string | null {
  if (!value) return null
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date.toISOString()
}

function fromApiDateTime(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 16)
}

function parseMetadata(): Record<string, unknown> | undefined {
  const raw = metadataDraft.value.trim()
  if (!raw) return {}
  try {
    const parsed: unknown = JSON.parse(raw)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) return parsed as Record<string, unknown>
  } catch {
    // The server will validate metadata too; keep the client error actionable.
  }
  appStore.showError(t('admin.activityCenter.invalidMetadata'))
  return undefined
}

function buildPayload(): ActivityCenterItemInput | null {
  const metadata = parseMetadata()
  if (!metadata) return null
  return {
    slug: form.slug.trim(),
    title: form.title.trim(),
    subtitle: form.subtitle.trim(),
    description: form.description.trim(),
    icon: form.icon.trim() || 'gift',
    cover_url: form.cover_url.trim(),
    route_path: form.route_path.trim(),
    external_url: form.external_url.trim(),
    action_label: form.action_label.trim(),
    activity_type: 'custom',
    enabled: form.enabled,
    sort_order: Number.isFinite(form.sort_order) ? Math.trunc(form.sort_order) : 0,
    start_at: toApiDateTime(form.start_at),
    end_at: toApiDateTime(form.end_at),
    metadata,
  }
}

async function saveItem(): Promise<void> {
  if (!canSaveItem.value) return
  const payload = buildPayload()
  if (!payload) return
  saving.value = true
  try {
    if (editingId.value === null) {
      await createActivityCenterItem(payload)
      appStore.showSuccess(t('admin.activityCenter.created'))
    } else {
      await updateActivityCenterItem(editingId.value, payload)
      appStore.showSuccess(t('admin.activityCenter.updated'))
    }
    resetForm()
    await loadItems()
  } catch (error) {
    const message = error instanceof Error ? error.message : undefined
    appStore.showError(message || t('admin.activityCenter.saveFailed'))
  } finally {
    saving.value = false
  }
}

function editItem(item: ActivityCenterItem): void {
  if (!config.enabled) return
  editingId.value = item.id
  form.slug = item.slug
  form.title = item.title
  form.subtitle = item.subtitle || ''
  form.description = item.description || ''
  form.icon = item.icon || 'gift'
  form.cover_url = item.cover_url || ''
  form.route_path = item.route_path || ''
  form.external_url = item.external_url || ''
  form.action_label = item.action_label || ''
  form.enabled = item.enabled
  form.sort_order = item.sort_order || 0
  form.start_at = fromApiDateTime(item.start_at)
  form.end_at = fromApiDateTime(item.end_at)
  metadataDraft.value = JSON.stringify(item.metadata || {}, null, 2)
}

function resetForm(): void {
  editingId.value = null
  Object.assign(form, emptyForm())
  metadataDraft.value = '{}'
}

function askDelete(item: ActivityCenterItem): void {
  if (!config.enabled || saving.value) return
  deleteDialog.id = item.id
  deleteDialog.title = item.title
  deleteDialog.show = true
}

async function confirmDelete(): Promise<void> {
  const id = deleteDialog.id
  deleteDialog.show = false
  if (!config.enabled || !id) return
  saving.value = true
  try {
    await deleteActivityCenterItem(id)
    appStore.showSuccess(t('admin.activityCenter.deleted'))
    if (editingId.value === id) resetForm()
    await loadItems()
  } catch (error) {
    const message = error instanceof Error ? error.message : undefined
    appStore.showError(message || t('admin.activityCenter.deleteFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void loadData()
})
</script>
