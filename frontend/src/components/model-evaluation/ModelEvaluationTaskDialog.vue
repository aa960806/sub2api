<template>
  <BaseDialog :show="show" :title="t(task ? 'modelEvaluations.admin.edit' : 'modelEvaluations.admin.create')" width="wide" @close="close">
    <form id="model-evaluation-task-form" class="space-y-4" @submit.prevent="save">
      <p class="rounded-xl bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-950/40 dark:text-blue-200">{{ t('modelEvaluations.admin.saveHelp') }}</p>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.name') }}</span><input v-model="form.name" name="name" class="input" maxlength="100" required /></label>
        <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.group') }}</span><select v-model.number="form.group_id" name="group_id" class="input" required><option :value="0" disabled>{{ t('modelEvaluations.admin.chooseGroup') }}</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label>
      </div>
      <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.format') }}</span><select v-model="form.api_format" name="api_format" class="input"><option value="chat_completions">{{ t('modelEvaluations.admin.chatCompletions') }}</option><option value="responses">{{ t('modelEvaluations.admin.responses') }}</option><option value="messages">{{ t('modelEvaluations.admin.anthropic') }}</option></select></label>
      <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.endpoint') }}</span><input v-model="form.endpoint" name="endpoint" type="url" pattern="https://.*" class="input" :placeholder="endpointExample" maxlength="2048" required :aria-invalid="!!endpointError" :aria-describedby="endpointError ? 'model-evaluation-endpoint-error' : undefined" @blur="endpointTouched = true" /><span class="block text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.endpointHelp') }}</span><span class="block break-all text-xs text-gray-500 dark:text-gray-400">{{ endpointExample }}</span><span v-if="endpointError" id="model-evaluation-endpoint-error" class="block text-xs text-red-600 dark:text-red-400" role="alert">{{ t(endpointError) }}</span></label>
      <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.key') }}</span><input v-model="form.api_key" name="api_key" type="password" autocomplete="new-password" class="input" maxlength="4096" :required="requiresKey" :placeholder="t(requiresKey ? 'modelEvaluations.admin.keyNew' : 'modelEvaluations.admin.keySaved')" /><span v-if="bindingChanged" class="block text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.keyBindingChanged') }}</span></label>
      <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.model') }}</span><input v-model="form.model" name="model" class="input" maxlength="200" required /><span class="block text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.modelHelp') }}</span></label>
      <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.reasoningEffort') }}</span><select v-model="form.reasoning_effort" name="reasoning_effort" class="input" data-testid="model-evaluation-reasoning-effort"><option value="">{{ t('modelEvaluations.admin.reasoningEffortDefault') }}</option><option v-for="effort in reasoningEfforts" :key="effort" :value="effort">{{ effort }}</option></select><span class="block text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.reasoningEffortHelp') }}</span><span v-if="legacyUltra" class="block text-xs text-amber-600 dark:text-amber-400">{{ t('modelEvaluations.admin.reasoningEffortLegacyUltra') }}</span></label>
      <div class="grid gap-4 sm:grid-cols-3">
        <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.interval') }}</span><input v-model.number="form.interval_seconds" name="interval_seconds" type="number" min="60" max="604800" step="1" class="input" required /></label>
        <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.retention') }}</span><input v-model.number="form.retention_days" name="retention_days" type="number" min="1" max="90" step="1" class="input" required /></label>
        <label class="block space-y-1 text-sm"><span>{{ t('modelEvaluations.admin.maxRecords') }}</span><input v-model.number="form.max_records" name="max_records" type="number" min="1" max="200" step="1" class="input" required /></label>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelEvaluations.admin.retentionHelp') }}</p>
      <label class="flex items-center gap-2 text-sm"><input v-model="form.enabled" name="enabled" type="checkbox" class="rounded" />{{ t('modelEvaluations.admin.taskEnabled') }}</label>
      <p class="rounded-xl bg-gray-100 p-3 text-sm dark:bg-dark-900"><span class="font-medium">{{ t('modelEvaluations.prompt') }}：</span>{{ MODEL_EVALUATION_PROMPT }}</p>
      <p v-if="error" class="text-sm text-red-600 dark:text-red-400" role="alert">{{ error }}</p>
    </form>
    <template #footer><button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('modelEvaluations.admin.cancel') }}</button><button type="submit" form="model-evaluation-task-form" class="btn btn-primary" :disabled="saving || !form.group_id">{{ t('modelEvaluations.admin.save') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminModelEvaluationsAPI, MODEL_EVALUATION_PROMPT, type ModelEvaluationGroup, type ModelEvaluationTask, type ModelEvaluationTaskInput, type ModelEvaluationReasoningEffort } from '@/api/modelEvaluations'
import { modelEvaluationEndpointErrorKey, modelEvaluationErrorKey } from '@/utils/modelEvaluationErrors'
const props = defineProps<{ show: boolean; task: ModelEvaluationTask | null; groups: ModelEvaluationGroup[] }>()
const emit = defineEmits<{ close: []; saved: [task: ModelEvaluationTask] }>()
const { t } = useI18n()
const reasoningEfforts: ModelEvaluationReasoningEffort[] = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max']
const legacyUltra = computed(() => props.task?.reasoning_effort === 'ultra')
function normalizeReasoningEffort(value?: string): ModelEvaluationReasoningEffort | '' {
  if (value === 'ultra') return 'max'
  return reasoningEfforts.includes(value as ModelEvaluationReasoningEffort) ? value as ModelEvaluationReasoningEffort : ''
}
const initial = (): ModelEvaluationTaskInput => ({ name: '', group_id: 0, endpoint: '', api_format: 'chat_completions', api_key: '', model: '', reasoning_effort: '', enabled: true, interval_seconds: 3600, retention_days: 7, max_records: 50 })
const form = reactive(initial())
const bindingChanged = computed(() => !!props.task && (form.endpoint.trim() !== props.task.endpoint || form.api_format !== props.task.api_format || form.group_id !== props.task.group_id))
const requiresKey = computed(() => !props.task?.has_api_key || bindingChanged.value)
const saving = ref(false)
const error = ref('')
const endpointTouched = ref(false)
const endpointError = computed(() => endpointTouched.value ? modelEvaluationEndpointErrorKey(form.endpoint) : '')
const endpointExample = computed(() => form.api_format === 'messages' ? 'https://api.example.com/v1/messages' : form.api_format === 'responses' ? 'https://api.example.com/v1/responses' : 'https://api.example.com/v1/chat/completions')
watch(() => props.show, show => {
  error.value = ''
  endpointTouched.value = false
  if (!show) { form.api_key = ''; return }
  const task = props.task
  Object.assign(form, task ? { name: task.name, group_id: task.group_id, endpoint: task.endpoint, api_format: task.api_format, api_key: '', model: task.model, reasoning_effort: normalizeReasoningEffort(task.reasoning_effort), enabled: task.enabled, interval_seconds: task.interval_seconds, retention_days: task.retention_days, max_records: task.max_records } : initial())
}, { immediate: true })
function close() { if (!saving.value) { form.api_key = ''; emit('close') } }
async function save() {
  if (saving.value) return
  endpointTouched.value = true
  if (endpointError.value) return
  if (requiresKey.value && !form.api_key?.trim()) { error.value = t('modelEvaluations.admin.keyRequired'); return }
  if (saving.value || !form.group_id) return
  saving.value = true
  error.value = ''
  try {
    const input = { ...form, api_key: form.api_key || undefined }
    const saved = props.task
      ? await adminModelEvaluationsAPI.update(props.task.id, input)
      : await adminModelEvaluationsAPI.create(input)
    form.api_key = ''
    emit('saved', saved)
  } catch (cause) { error.value = t(modelEvaluationErrorKey(cause)) }
  finally { saving.value = false }
}
</script>
