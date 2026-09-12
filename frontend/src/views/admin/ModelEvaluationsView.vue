<template>
  <AppLayout>
    <div class="space-y-6">
      <header class="card space-y-3 p-5 sm:p-6">
        <h1 class="page-title">{{ t('modelEvaluations.title') }}</h1>
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('modelEvaluations.admin.description') }}</p>
        <label class="flex items-center gap-2 font-medium"><input :checked="enabled" :disabled="loading || configSaving || loadError" type="checkbox" data-testid="monitor-enabled" @change="setEnabled" />{{ t('modelEvaluations.admin.globalEnabled') }}</label>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.globalHelp') }}</p>
        <p class="rounded-xl bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-950/40 dark:text-blue-200">{{ t('modelEvaluations.admin.workflow') }}</p>
      </header>
      <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-lg font-semibold">{{ t('modelEvaluations.admin.tasks') }}</h2><div class="flex flex-wrap gap-2"><button type="button" class="btn btn-secondary" :disabled="loading" @click="load">{{ t('modelEvaluations.refresh') }}</button><button type="button" class="btn btn-secondary" :disabled="loading || loadError" @click="openCleanup">{{ t('modelEvaluations.admin.cleanup') }}</button><button type="button" class="btn btn-primary" :disabled="loading || loadError || tasks.length >= 100" @click="openTask(null)">{{ t('modelEvaluations.admin.create') }}</button></div></div>
      <p v-if="loading" role="status">{{ t('modelEvaluations.loading') }}</p>
      <p v-else-if="loadError" role="alert">{{ t('modelEvaluations.loadFailed') }}</p>
      <p v-else-if="!tasks.length" class="card p-6 text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.noTasks') }}</p>
      <div v-else class="grid gap-4 lg:grid-cols-2">
        <article v-for="task in tasks" :key="task.id" class="card min-w-0 space-y-3 p-5">
          <div class="flex items-center justify-between gap-2"><h3 class="truncate font-semibold">{{ task.name }}</h3><span class="text-xs" :class="task.published === true ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-500'">{{ t(task.published === true ? 'modelEvaluations.admin.published' : 'modelEvaluations.admin.unpublished') }}</span></div>
          <p class="truncate text-sm">{{ task.group_name }} · {{ task.model }}</p>
          <p class="truncate text-xs text-gray-500 dark:text-gray-400" :title="task.endpoint">{{ task.endpoint }}</p>
          <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400"><span>{{ t('modelEvaluations.admin.interval') }}: {{ task.interval_seconds }}</span><span>{{ t('modelEvaluations.admin.retention') }}: {{ task.retention_days }}</span><span>{{ t('modelEvaluations.admin.maxRecords') }}: {{ task.max_records }}</span></div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.nextRun') }}: {{ enabled && task.enabled && task.published === true && task.next_run_at ? new Date(task.next_run_at).toLocaleString(locale) : '—' }}</p>
          <p class="text-sm" data-testid="test-status">{{ t('modelEvaluations.admin.testStatus') }}: {{ t(testStatusKey(task)) }}</p>
          <p v-if="task.last_tested_at" class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.lastTestedAt') }}: {{ new Date(task.last_tested_at).toLocaleString(locale) }}</p>
          <p v-if="task.test_status === 'failed' && task.test_error" class="max-h-32 overflow-y-auto break-words rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300" data-testid="test-error">{{ t('modelEvaluations.admin.testError') }}: {{ task.test_error }}</p>
          <div class="flex flex-wrap gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="taskBusy(task)" @click="openTask(task)">{{ t('modelEvaluations.admin.edit') }}</button>
            <button type="button" class="btn btn-primary btn-sm" :disabled="taskBusy(task)" @click="testTask(task)">{{ t('modelEvaluations.admin.test') }}</button>
            <button v-if="task.published === true" type="button" class="btn btn-secondary btn-sm" :disabled="publicationIds.has(task.id)" @click="setPublication(task, false)">{{ t('modelEvaluations.admin.unpublish') }}</button>
            <button v-else type="button" class="btn btn-secondary btn-sm" :disabled="task.test_status !== 'passed' || taskBusy(task)" :title="task.test_status !== 'passed' ? t('modelEvaluations.admin.testRequired') : undefined" @click="setPublication(task, true)">{{ t('modelEvaluations.admin.publish') }}</button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="!enabled || !task.enabled || task.published !== true || taskBusy(task)" @click="run(task)">{{ t('modelEvaluations.admin.run') }}</button>
            <button type="button" class="btn btn-ghost btn-sm text-red-600 dark:text-red-400" @click="pendingDelete = task">{{ t('modelEvaluations.admin.delete') }}</button>
          </div>
        </article>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.taskLimit') }}</p>
      <p v-if="pollPaused" role="status" class="text-sm text-amber-700 dark:text-amber-300">{{ t('modelEvaluations.admin.pollPaused') }}</p>
      <section v-if="!loading && !loadError" class="space-y-4"><h2 class="text-lg font-semibold">{{ t('modelEvaluations.admin.history') }}</h2><ModelEvaluationGallery :groups="monitoredGroups" admin :revision="revision" /></section>
    </div>
    <ModelEvaluationTaskDialog :show="taskOpen" :task="editingTask" :groups="groups" @close="taskOpen = false" @saved="taskSaved" />
    <BaseDialog :show="pendingDelete !== null" :title="t('modelEvaluations.admin.deleteTask')" width="narrow" @close="pendingDelete = null"><p>{{ t('modelEvaluations.admin.deleteTaskConfirm', { name: pendingDelete?.name }) }}</p><template #footer><button type="button" class="btn btn-secondary" :disabled="deleting" @click="pendingDelete = null">{{ t('modelEvaluations.admin.cancel') }}</button><button type="button" class="btn btn-danger" :disabled="deleting" @click="deleteTask">{{ t('modelEvaluations.admin.delete') }}</button></template></BaseDialog>
    <BaseDialog :show="cleanupOpen" :title="t('modelEvaluations.admin.cleanup')" @close="cleanupOpen = false">
      <div class="space-y-4"><label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.cleanupScope') }}</span><select v-model.number="cleanupTaskId" class="input"><option :value="0">{{ t('modelEvaluations.admin.allTasks') }}</option><option v-for="task in tasks" :key="task.id" :value="task.id">{{ task.name }}</option></select></label><label class="flex items-center gap-2 text-sm"><input v-model="cleanupAll" type="checkbox" />{{ t('modelEvaluations.admin.cleanupAll') }}</label><p class="text-sm text-gray-500 dark:text-gray-400">{{ t(cleanupAll ? 'modelEvaluations.admin.cleanupConfirm' : 'modelEvaluations.admin.retentionHelp') }}</p></div>
      <template #footer><button type="button" class="btn btn-secondary" :disabled="cleaning" @click="cleanupOpen = false">{{ t('modelEvaluations.admin.cancel') }}</button><button type="button" :class="['btn', cleanupAll ? 'btn-danger' : 'btn-primary']" :disabled="cleaning" @click="cleanup">{{ t(cleanupAll ? 'modelEvaluations.admin.cleanupAll' : 'modelEvaluations.admin.cleanupExpired') }}</button></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ModelEvaluationGallery from '@/components/model-evaluation/ModelEvaluationGallery.vue'
import ModelEvaluationTaskDialog from '@/components/model-evaluation/ModelEvaluationTaskDialog.vue'
import { adminModelEvaluationsAPI, type ModelEvaluationTask, type ModelEvaluationGroup } from '@/api/modelEvaluations'
import { getAllIncludingInactive } from '@/api/admin/groups'
import { useAppStore } from '@/stores/app'
import { modelEvaluationErrorKey } from '@/utils/modelEvaluationErrors'
const { t, locale } = useI18n()
const appStore = useAppStore()
const enabled = ref(false)
const configSaving = ref(false)
const tasks = ref<ModelEvaluationTask[]>([])
const groups = ref<ModelEvaluationGroup[]>([])
const loading = ref(true)
const loadError = ref(false)
const taskOpen = ref(false)
const editingTask = ref<ModelEvaluationTask | null>(null)
const pendingDelete = ref<ModelEvaluationTask | null>(null)
const deleting = ref(false)
const cleanupOpen = ref(false)
const cleanupTaskId = ref(0)
const cleanupAll = ref(false)
const cleaning = ref(false)
const revision = ref(0)
const runningIds = ref(new Set<number>())
const testingIds = ref(new Set<number>())
const publicationIds = ref(new Set<number>())
const pollPaused = ref(false)
let taskController: AbortController | null = null
let taskGeneration = 0
let disposed = false
let pollTimer: ReturnType<typeof setTimeout> | undefined
let pollCount = 0
// Backend leases permit requests up to 660s (plus a small persistence margin).
// Poll for 12 minutes at 2s intervals so long-running tests remain visibly
// pending instead of appearing complete; hidden/unmounted cancellation remains unchanged.
const MAX_STATUS_POLLS = 360
const monitoredGroups = computed(() => [...new Map(tasks.value.map(task => [task.group_id, { id: task.group_id, name: task.group_name }])).values()])
function testStatusKey(task: ModelEvaluationTask) {
  const status = task.test_status
  return `modelEvaluations.admin.${status === 'passed' ? 'passed' : status === 'failed' ? 'failedTest' : status === 'running' ? 'running' : 'untested'}`
}
function taskBusy(task: ModelEvaluationTask) {
  return task.test_status === 'running' || runningIds.value.has(task.id) || testingIds.value.has(task.id) || publicationIds.value.has(task.id)
}
function stopStatusPolling() { clearTimeout(pollTimer); pollTimer = undefined }
function scheduleStatusPoll() {
  stopStatusPolling()
  if (disposed || document.hidden || !tasks.value.some(task => task.test_status === 'running')) return
  if (pollCount >= MAX_STATUS_POLLS) { pollPaused.value = true; return }
  pollTimer = setTimeout(() => {
    pollTimer = undefined
    pollCount++
    void refreshTasks().catch(() => { pollPaused.value = true })
  }, 2000)
}
async function refreshTasks(resetBudget = false) {
  stopStatusPolling()
  taskController?.abort()
  const controller = new AbortController()
  taskController = controller
  const generation = ++taskGeneration
  if (resetBudget) { pollCount = 0; pollPaused.value = false }
  const response = await adminModelEvaluationsAPI.tasks(controller.signal)
  if (disposed || controller.signal.aborted || generation !== taskGeneration) return
  const finished = tasks.value.some(task => task.test_status === 'running' && response.items.find(next => next.id === task.id)?.test_status !== 'running')
  tasks.value = response.items
  if (finished) revision.value++
  scheduleStatusPoll()
}
function visibilityChanged() {
  if (document.hidden) {
    stopStatusPolling()
    taskController?.abort()
    taskGeneration++
  } else scheduleStatusPoll()
}
async function load() {
  loading.value = true
  loadError.value = false
  try {
    const [config, , allGroups] = await Promise.all([adminModelEvaluationsAPI.config(), refreshTasks(true), getAllIncludingInactive()])
    if (disposed) return
    enabled.value = config.enabled
    groups.value = allGroups.map(group => ({ id: group.id, name: group.name }))
    revision.value++
  } catch { if (!disposed) loadError.value = true }
  finally { loading.value = false }
}
async function setEnabled(event: Event) {
  const input = event.target as HTMLInputElement
  configSaving.value = true
  try {
    const config = await adminModelEvaluationsAPI.setConfig(input.checked)
    enabled.value = config.enabled
    appStore.showSuccess(t('modelEvaluations.admin.saved'))
    await appStore.fetchPublicSettings(true)
  } catch (cause) { input.checked = enabled.value; appStore.showError(t(modelEvaluationErrorKey(cause))) }
  finally { configSaving.value = false }
}
function openTask(task: ModelEvaluationTask | null) { editingTask.value = task; taskOpen.value = true }
function openCleanup() { cleanupTaskId.value = 0; cleanupAll.value = false; cleanupOpen.value = true }
async function taskSaved(task: ModelEvaluationTask) {
  taskOpen.value = false
  appStore.showSuccess(t(task?.published === true ? 'modelEvaluations.admin.saved' : 'modelEvaluations.admin.savedDraft'))
  await load()
}
async function deleteTask() {
  if (!pendingDelete.value || deleting.value) return
  deleting.value = true
  try { await adminModelEvaluationsAPI.deleteTask(pendingDelete.value.id); pendingDelete.value = null; appStore.showSuccess(t('modelEvaluations.admin.deleted')); await load() }
  catch (cause) { appStore.showError(t(modelEvaluationErrorKey(cause))) }
  finally { deleting.value = false }
}
async function cleanup() {
  if (cleaning.value) return
  cleaning.value = true
  try {
    const result = await adminModelEvaluationsAPI.cleanup({ task_id: cleanupTaskId.value || undefined, all: cleanupAll.value })
    cleanupOpen.value = false
    appStore.showSuccess(t('modelEvaluations.admin.cleanupDone', { count: result.deleted }))
    revision.value++
  } catch (cause) { appStore.showError(t(modelEvaluationErrorKey(cause))) }
  finally { cleaning.value = false }
}
async function run(task: ModelEvaluationTask) {
  if (!enabled.value || !task.enabled || task.published !== true || taskBusy(task)) return
  runningIds.value.add(task.id)
  try { await adminModelEvaluationsAPI.run(task.id); appStore.showSuccess(t('modelEvaluations.admin.runAccepted')) }
  catch (cause) { appStore.showError(t(modelEvaluationErrorKey(cause))) }
  finally { runningIds.value.delete(task.id) }
}
async function testTask(task: ModelEvaluationTask) {
  if (taskBusy(task)) return
  testingIds.value.add(task.id)
  stopStatusPolling()
  taskController?.abort()
  taskGeneration++
  try {
    await adminModelEvaluationsAPI.test(task.id)
    task.test_status = 'running'
    task.test_error = ''
    appStore.showSuccess(t('modelEvaluations.admin.testAccepted'))
    await refreshTasks(true)
  } catch (cause) { appStore.showError(t(modelEvaluationErrorKey(cause))); scheduleStatusPoll() }
  finally { testingIds.value.delete(task.id) }
}
async function setPublication(task: ModelEvaluationTask, published: boolean) {
  if (publicationIds.value.has(task.id) || (published && (task.test_status !== 'passed' || taskBusy(task)))) return
  publicationIds.value.add(task.id)
  stopStatusPolling()
  taskController?.abort()
  taskGeneration++
  try {
    const updated = await adminModelEvaluationsAPI.setPublication(task.id, published)
    tasks.value = tasks.value.map(current => current.id === task.id ? updated : current)
    appStore.showSuccess(t(published ? 'modelEvaluations.admin.publishedSuccess' : 'modelEvaluations.admin.unpublishedSuccess'))
    revision.value++
  } catch (cause) { appStore.showError(t(modelEvaluationErrorKey(cause))) }
  finally { publicationIds.value.delete(task.id); scheduleStatusPoll() }
}
onMounted(() => { document.addEventListener('visibilitychange', visibilityChanged); void load() })
onBeforeUnmount(() => {
  disposed = true
  stopStatusPolling()
  taskController?.abort()
  taskGeneration++
  document.removeEventListener('visibilitychange', visibilityChanged)
})
</script>
