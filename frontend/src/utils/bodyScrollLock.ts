const activeLocks = new Set<symbol>()
let previousOverflow: string | null = null

function acquire(token: symbol): void {
  if (typeof document === 'undefined' || activeLocks.has(token)) return
  if (activeLocks.size === 0) previousOverflow = document.body.style.overflow
  activeLocks.add(token)
  document.body.style.overflow = 'hidden'
}

function release(token: symbol): void {
  if (typeof document === 'undefined' || !activeLocks.delete(token)) return
  if (activeLocks.size > 0) return
  document.body.style.overflow = previousOverflow ?? ''
  previousOverflow = null
}

export function createBodyScrollLock(): { set: (locked: boolean) => void; release: () => void } {
  const token = Symbol('body-scroll-lock')
  return {
    set(locked: boolean) {
      if (locked) acquire(token)
      else release(token)
    },
    release() {
      release(token)
    },
  }
}
