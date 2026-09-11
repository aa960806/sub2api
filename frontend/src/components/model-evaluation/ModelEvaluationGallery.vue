<template>
  <section class="space-y-4" :aria-label="t('modelEvaluations.admin.history')">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <label class="flex min-w-0 items-center gap-2 text-sm">
        <span class="shrink-0 whitespace-nowrap">{{ t('modelEvaluations.group') }}</span>
        <select v-model.number="groupId" class="input min-w-0 max-w-64" data-testid="group-filter">
          <option :value="0">{{ t('modelEvaluations.allGroups') }}</option>
          <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option>
        </select>
      </label>
      <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadResults">{{ t('modelEvaluations.refresh') }}</button>
    </div>
    <p v-if="!admin" class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.newestFirst') }}</p>
    <p v-if="loading" role="status" class="py-12 text-center text-gray-500">{{ t('modelEvaluations.loading') }}</p>
    <div v-else-if="error" role="alert" class="card p-8 text-center">
      <p>{{ t('modelEvaluations.loadFailed') }}</p>
      <button type="button" class="btn btn-secondary mt-3" @click="loadResults">{{ t('modelEvaluations.retry') }}</button>
    </div>
    <p v-else-if="!results.length" class="card user-glass-panel p-10 text-center text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.empty') }}</p>
    <div v-else class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
      <article v-for="result in results" :key="result.id" :ref="el => observeCard(el, result.id)" class="card user-glass-panel flex min-w-0 flex-col overflow-hidden" :data-result-id="result.id">
        <header class="space-y-2 p-4" :class="{ 'order-last': admin }">
          <div class="flex items-center justify-between gap-2"><h3 class="min-w-0 truncate font-semibold" :title="result.group_name">{{ result.group_name }}</h3><span class="shrink-0 rounded-full px-2 py-0.5 text-xs" :class="result.status === 'success' ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300' : 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300'">{{ t(`modelEvaluations.${result.status}`) }}</span></div>
          <div v-if="!admin" class="flex flex-wrap justify-between gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400"><span>{{ t('modelEvaluations.generatedAt') }}: <time :datetime="result.created_at">{{ formatTime(result.created_at) }}</time></span><span>{{ t('modelEvaluations.duration', { seconds: (result.duration_ms / 1000).toFixed(1) }) }}</span></div>
          <p class="truncate text-sm text-gray-600 dark:text-gray-300" :title="result.model">{{ result.model }}</p>
          <p class="truncate text-xs text-gray-500 dark:text-gray-400" :title="result.task_name">{{ result.task_name }}</p>
          <p v-if="admin && result.is_test" class="text-xs text-amber-600 dark:text-amber-400">{{ t('modelEvaluations.admin.privateTest') }}</p>
          <div v-if="admin" class="flex flex-wrap justify-between gap-1 text-xs text-gray-500 dark:text-gray-400"><time :datetime="result.created_at">{{ formatTime(result.created_at) }}</time><span>{{ t('modelEvaluations.duration', { seconds: (result.duration_ms / 1000).toFixed(1) }) }}</span></div>
          <button v-if="admin" type="button" class="text-xs text-red-600 hover:underline dark:text-red-400" @click="pendingDelete = result.id">{{ t('modelEvaluations.admin.deleteResult') }}</button>
        </header>
        <button v-if="result.status === 'success'" type="button" class="relative block h-52 w-full overflow-hidden bg-slate-950 text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500" :aria-label="`${t('modelEvaluations.preview')}: ${result.group_name} · ${result.model}`" @mouseenter="scheduleHover(result.id)" @mouseleave="closeHover" @focus="scheduleHover(result.id)" @blur="closeHover" @click="openPreview(result.id)">
          <ModelEvaluationPreview v-if="htmlById[result.id] && smallActiveIds.has(result.id)" :html="htmlById[result.id]!" :title="result.model" class="pointer-events-none h-full w-full" />
          <span v-else class="flex h-full items-center justify-center px-4 text-sm text-slate-300">{{ detailErrors.has(result.id) ? t('modelEvaluations.previewFailed') : t('modelEvaluations.previewPaused') }}</span>
          <span class="pointer-events-none absolute inset-x-0 bottom-0 bg-black/65 px-3 py-2 text-xs">{{ t('modelEvaluations.previewHint') }}</span>
        </button>
        <div v-else class="flex h-52 items-center justify-center bg-red-50 p-5 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">
          <p class="max-h-36 overflow-y-auto break-words">{{ admin && result.error_message ? result.error_message : t('modelEvaluations.generationFailed') }}</p>
        </div>
      </article>
    </div>
    <nav v-if="total > pageSize" class="flex items-center justify-between gap-3" :aria-label="t('modelEvaluations.admin.history')">
      <button type="button" class="btn btn-secondary" :disabled="loading || page <= 1" @click="page--">{{ t('modelEvaluations.previous') }}</button>
      <span class="text-sm">{{ t('modelEvaluations.page', { page, total: totalPages }) }}</span>
      <button type="button" class="btn btn-secondary" :disabled="loading || page >= totalPages" @click="page++">{{ t('modelEvaluations.next') }}</button>
    </nav>
    <Teleport to="body">
      <div v-if="hoverId && !pinnedId && documentVisible" class="pointer-events-none fixed left-1/2 top-1/2 z-[80] hidden h-[65vh] w-[min(80vw,900px)] -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-2xl border border-white/20 bg-slate-950 shadow-2xl sm:block" aria-hidden="true" data-testid="hover-preview">
        <ModelEvaluationPreview v-if="htmlById[hoverId]" :html="htmlById[hoverId]!" class="h-full w-full" />
        <p v-else class="p-8 text-white">{{ t(detailErrors.has(hoverId) ? 'modelEvaluations.previewFailed' : 'modelEvaluations.loading') }}</p>
      </div>
    </Teleport>
    <BaseDialog :show="pinnedId !== null" :title="previewTitle" width="extra-wide" :z-index="90" close-on-click-outside @close="closePinned">
      <div class="h-[65vh] min-h-64 overflow-hidden rounded-xl bg-slate-950">
        <ModelEvaluationPreview v-if="pinnedId && htmlById[pinnedId] && documentVisible" :html="htmlById[pinnedId]!" :title="previewTitle" class="h-full w-full" />
        <div v-else class="p-8 text-white"><p>{{ t(pinnedId && detailErrors.has(pinnedId) ? 'modelEvaluations.previewFailed' : 'modelEvaluations.loading') }}</p><button v-if="pinnedId && detailErrors.has(pinnedId)" type="button" class="btn btn-secondary mt-3" @click="loadHTML(pinnedId, true)">{{ t('modelEvaluations.retry') }}</button></div>
      </div>
    </BaseDialog>
    <BaseDialog :show="pendingDelete !== null" :title="t('modelEvaluations.admin.deleteResult')" width="narrow" @close="pendingDelete = null">
      <p>{{ t('modelEvaluations.admin.deleteResultConfirm') }}</p>
      <template #footer><button type="button" class="btn btn-secondary" :disabled="deleting" @click="pendingDelete = null">{{ t('modelEvaluations.admin.cancel') }}</button><button type="button" class="btn btn-danger" :disabled="deleting" @click="deleteResult">{{ t('modelEvaluations.admin.delete') }}</button></template>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminModelEvaluationsAPI, modelEvaluationsAPI, type ModelEvaluationGroup, type ModelEvaluationResult } from '@/api/modelEvaluations'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ModelEvaluationPreview from './ModelEvaluationPreview.vue'

const props = withDefaults(defineProps<{ groups: ModelEvaluationGroup[]; admin?: boolean; revision?: number }>(), { admin: false, revision: 0 })
const { t, locale } = useI18n()
const appStore = useAppStore()
const pageSize = 12
const groupId = ref(0)
const page = ref(1)
const total = ref(0)
const results = ref<ModelEvaluationResult[]>([])
const loading = ref(false)
const error = ref(false)
const htmlById = ref<Record<number, string>>({})
const detailErrors = ref(new Set<number>())
const visibleIds = ref(new Set<number>())
const documentVisible = ref(!document.hidden)
const hoverId = ref<number | null>(null)
const pinnedId = ref<number | null>(null)
const pendingDelete = ref<number | null>(null)
const deleting = ref(false)
let generation = 0
let controller: AbortController | null = null
let hoverTimer: ReturnType<typeof setTimeout> | undefined
let suppressFocus = false
const pendingDetails = new Set<number>()
const cardElements = new Map<Element, number>()
let observer: IntersectionObserver | null = null
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const previewTitle = computed(() => {
  const selected = results.value.find(result => result.id === pinnedId.value)
  return selected ? `${selected.group_name} · ${selected.model}` : t('modelEvaluations.preview')
})
const smallActiveIds = computed(() => new Set(documentVisible.value && !pinnedId.value && !hoverId.value
  ? results.value.filter(result => result.status === 'success' && visibleIds.value.has(result.id)).slice(0, 2).map(result => result.id)
  : []))

function formatTime(value: string) { return new Date(value).toLocaleString(locale.value) }
function observeCard(element: Element | ComponentPublicInstance | null, id: number) {
  if (!(element instanceof Element) || cardElements.has(element)) return
  cardElements.set(element, id)
  observer?.observe(element)
}
async function loadResults() {
  const current = ++generation
  controller?.abort()
  controller = new AbortController()
  observer?.disconnect()
  cardElements.clear()
  closeHover()
  pinnedId.value = null
  results.value = []
  htmlById.value = {}
  detailErrors.value = new Set()
  visibleIds.value = new Set()
  pendingDetails.clear()
  loading.value = true
  error.value = false
  try {
    const response = await modelEvaluationsAPI.results({ group_id: groupId.value || undefined, page: page.value, page_size: pageSize }, controller.signal, props.admin)
    if (current !== generation) return
    results.value = response.items
    total.value = response.total
    if (page.value > totalPages.value) page.value = totalPages.value
  } catch {
    if (current === generation && !controller.signal.aborted) error.value = true
  } finally {
    if (current === generation) {
      loading.value = false
      await nextTick()
      if (!observer) visibleIds.value = new Set(results.value.slice(0, 2).map(result => result.id))
    }
  }
}
async function loadHTML(id: number, retry = false) {
  if (htmlById.value[id] || pendingDetails.has(id) || (!retry && detailErrors.value.has(id))) return
  const current = generation
  const signal = controller?.signal
  pendingDetails.add(id)
  detailErrors.value.delete(id)
  try {
    const result = await modelEvaluationsAPI.result(id, signal, props.admin)
    if (current !== generation || signal?.aborted) return
    if (result.html) htmlById.value[id] = result.html
    else detailErrors.value.add(id)
  } catch {
    if (current === generation && !signal?.aborted) detailErrors.value.add(id)
  } finally {
    if (current === generation) pendingDetails.delete(id)
  }
}
function scheduleHover(id: number) {
  if (suppressFocus || pinnedId.value || !window.matchMedia('(hover: hover) and (min-width: 640px)').matches) return
  clearTimeout(hoverTimer)
  hoverTimer = setTimeout(() => { hoverId.value = id; void loadHTML(id) }, 250)
}
function closeHover() { clearTimeout(hoverTimer); hoverId.value = null }
function openPreview(id: number) { closeHover(); pinnedId.value = id; void loadHTML(id, true) }
function closePinned() {
  suppressFocus = true
  pinnedId.value = null
  void nextTick(() => { suppressFocus = false })
}
function visibilityChanged() {
  documentVisible.value = !document.hidden
  if (document.hidden) closeHover()
}
async function deleteResult() {
  if (pendingDelete.value === null || deleting.value) return
  deleting.value = true
  try {
    await adminModelEvaluationsAPI.deleteResult(pendingDelete.value)
    pendingDelete.value = null
    appStore.showSuccess(t('modelEvaluations.admin.deleted'))
    await loadResults()
  } catch { appStore.showError(t('modelEvaluations.admin.failed')) }
  finally { deleting.value = false }
}
watch(smallActiveIds, ids => { for (const id of ids) void loadHTML(id) })
watch(groupId, () => { if (page.value !== 1) page.value = 1; else void loadResults() })
watch(page, () => void loadResults())
watch(() => props.revision, () => void loadResults())
watch(() => props.groups, groups => { if (groupId.value && !groups.some(group => group.id === groupId.value)) groupId.value = 0 })
onMounted(() => {
  if (typeof IntersectionObserver !== 'undefined') observer = new IntersectionObserver(entries => {
    const next = new Set(visibleIds.value)
    for (const entry of entries) {
      const id = cardElements.get(entry.target)
      if (id !== undefined) { if (entry.isIntersecting) next.add(id); else next.delete(id) }
    }
    visibleIds.value = next
  }, { threshold: 0.1 })
  document.addEventListener('visibilitychange', visibilityChanged)
  void loadResults()
})
onBeforeUnmount(() => {
  generation++
  controller?.abort()
  observer?.disconnect()
  clearTimeout(hoverTimer)
  document.removeEventListener('visibilitychange', visibilityChanged)
})
</script>
