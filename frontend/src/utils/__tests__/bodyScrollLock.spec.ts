import { afterEach, describe, expect, it } from 'vitest'
import { createBodyScrollLock } from '@/utils/bodyScrollLock'

describe('body scroll lock', () => {
  afterEach(() => {
    document.body.style.overflow = ''
  })

  it('keeps scrolling locked until every modal releases its lock', () => {
    document.body.style.overflow = 'clip'
    const first = createBodyScrollLock()
    const second = createBodyScrollLock()

    first.set(true)
    second.set(true)
    first.release()
    expect(document.body.style.overflow).toBe('hidden')

    second.set(false)
    expect(document.body.style.overflow).toBe('clip')
  })

  it('treats repeated state updates and releases as idempotent', () => {
    const lock = createBodyScrollLock()

    lock.set(true)
    lock.set(true)
    expect(document.body.style.overflow).toBe('hidden')
    lock.release()
    lock.release()
    expect(document.body.style.overflow).toBe('')
  })
})
