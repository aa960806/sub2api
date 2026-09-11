import { describe, expect, it } from 'vitest'
import { modelEvaluationEndpointErrorKey, modelEvaluationErrorKey } from '../modelEvaluationErrors'
describe('model evaluation messages', () => {
  it('uses only known safe error codes and ignores arbitrary server messages', () => {
    expect(modelEvaluationErrorKey({ code: 'MODEL_EVALUATION_TEST_REQUIRED', message: 'secret' })).toBe('modelEvaluations.admin.testRequired')
    expect(modelEvaluationErrorKey({ message: 'secret', code: 'UNKNOWN' })).toBe('modelEvaluations.admin.failed')
    expect(modelEvaluationErrorKey({ reason: 'MODEL_EVALUATION_BUSY' })).toBe('modelEvaluations.admin.busy')
  })
  it('requires a complete HTTPS path while retaining custom gateway paths', () => {
    expect(modelEvaluationEndpointErrorKey('https://example.com')).toBe('modelEvaluations.admin.endpointPathRequired')
    expect(modelEvaluationEndpointErrorKey('http://example.com/v1/messages')).toBe('modelEvaluations.admin.endpointInvalid')
    expect(modelEvaluationEndpointErrorKey('https://user:key@example.com/v1/messages')).toBe('modelEvaluations.admin.endpointInvalid')
    expect(modelEvaluationEndpointErrorKey('https://example.com/api/custom/messages')).toBe('')
  })
})
