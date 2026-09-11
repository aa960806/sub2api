// Only known server error codes select UI messages. Never display arbitrary
// provider/API error bodies, which may echo endpoint credentials or headers.
export function modelEvaluationErrorKey(error: unknown): string {
  const value = error as { code?: unknown; reason?: unknown } | null
  const keys: Record<string, string> = {
    MODEL_EVALUATION_INVALID: 'invalidConfiguration',
    MODEL_EVALUATION_CREDENTIAL_REQUIRED: 'keyRequired',
    MODEL_EVALUATION_BUSY: 'busy',
    MODEL_EVALUATION_LIMIT: 'taskLimit',
    MODEL_EVALUATION_DISABLED: 'monitorDisabled',
    MODEL_EVALUATION_NOT_FOUND: 'taskNotFound',
    MODEL_EVALUATION_TEST_REQUIRED: 'testRequired',
    MODEL_EVALUATION_NOT_PUBLISHED: 'publishRequired',
    MODEL_EVALUATION_ENDPOINT_PATH_REQUIRED: 'endpointPathRequired',
  }
  for (const code of [value?.reason, value?.code]) {
    if (typeof code === 'string' && keys[code]) return `modelEvaluations.admin.${keys[code]}`
  }
  return 'modelEvaluations.admin.failed'
}

export function modelEvaluationEndpointErrorKey(value: string): string {
  try {
    const url = new URL(value.trim())
    if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash) {
      return 'modelEvaluations.admin.endpointInvalid'
    }
    if (!url.pathname || url.pathname === '/') return 'modelEvaluations.admin.endpointPathRequired'
    return ''
  } catch {
    return 'modelEvaluations.admin.endpointInvalid'
  }
}
