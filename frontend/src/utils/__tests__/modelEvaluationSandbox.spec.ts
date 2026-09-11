import { describe, expect, it } from 'vitest'
import { createModelEvaluationDocument, MODEL_EVALUATION_CSP } from '../modelEvaluationSandbox'

function parse(html: string) {
  return new DOMParser().parseFromString(createModelEvaluationDocument(html), 'text/html')
}

describe('model evaluation sandbox document', () => {
  it('places restrictive resource policy ahead of model content', () => {
    const output = parse('<head><meta http-equiv="Content-Security-Policy" content="default-src *"><script>window.started = true</script></head>')
    const policies = output.querySelectorAll('meta[http-equiv]')
    expect(policies).toHaveLength(1)
    expect(policies[0].getAttribute('content')).toBe(MODEL_EVALUATION_CSP)
    expect(output.head.children[0].getAttribute('charset')).toBe('utf-8')
    expect(output.head.children[1]).toBe(policies[0])
    for (const directive of ["default-src 'none'", "connect-src 'none'", "form-action 'none'", "base-uri 'none'", "frame-src 'none'", "object-src 'none'", "worker-src 'none'"]) {
      expect(MODEL_EVALUATION_CSP).toContain(directive)
    }
    expect(output.querySelector('meta[name="referrer"]')?.getAttribute('content')).toBe('no-referrer')
  })

  it('preserves inline SVG, CSS, scripts, body attributes and local animation references', () => {
    const script = 'const wheel = document.getElementById("wheel"); requestAnimationFrame(() => wheel.dataset.moved = "yes");'
    const css = 'body { margin: 0; background: #112233 } @keyframes turn { to { transform: rotate(360deg) } } #wheel { animation: turn 2s linear infinite }'
    const output = parse(`<html lang="zh"><head><style>${css}</style></head><body class="scene" style="display:grid"><svg viewBox="0 0 960 600"><defs><circle id="wheel" r="40"/></defs><use href="#wheel"/><animate href="#wheel" attributeName="r" values="40;45;40" dur="1s" repeatCount="indefinite"/></svg><script>${script}</script></body></html>`)
    expect(output.documentElement.lang).toBe('zh')
    expect(output.body.className).toBe('scene')
    expect(output.body.getAttribute('style')).toBe('display:grid')
    expect(output.querySelector('style')?.textContent).toBe(css)
    expect(output.querySelector('script')?.textContent).toBe(script)
    expect(output.querySelector('svg')?.getAttribute('viewBox')).toBe('0 0 960 600')
    expect(output.querySelector('use')?.getAttribute('href')).toBe('#wheel')
    expect(output.querySelector('animate')?.getAttribute('repeatCount')).toBe('indefinite')
  })

  it('removes static navigation and external executable/embedded resources', () => {
    const output = parse(`<base href="https://example.invalid/"><meta HTTP-EQUIV="refresh" content="0;url=https://example.invalid"><link rel="stylesheet" href="https://example.invalid/style.css"><iframe srcdoc="unsafe"></iframe><object data="https://example.invalid"></object><embed src="https://example.invalid"><script src="https://example.invalid/a.js"></script><svg><script href="https://example.invalid/b.js"/></svg><a href="javascript:location.href='https://example.invalid'" ping="https://example.invalid" target="_top">link</a><form action="https://example.invalid" target="_top"><button formaction="https://example.invalid" formtarget="_blank">submit</button></form>`)
    expect(output.querySelector('base, link, iframe, object, embed, script')).toBeNull()
    expect(output.querySelectorAll('meta[http-equiv]')).toHaveLength(1)
    expect(output.querySelector('a')?.getAttributeNames()).toEqual([])
    expect(output.querySelector('form')?.getAttributeNames()).toEqual([])
    expect(output.querySelector('button')?.getAttributeNames()).toEqual([])
  })

  it('allows embedded images and SVG fragment references but strips remote and relative resources', () => {
    const image = 'data:image/png;base64,aGVsbG8='
    const output = parse(`<img id="data" src="${image}"><img id="remote" src="https://example.invalid/a.png" srcset="https://example.invalid/b.png 2x"><img id="relative" src="/api/private"><svg xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="#wheel"/><image id="embedded" href="${image}"/><image id="external" href="https://example.invalid/a.svg"/></svg>`)
    expect(output.querySelector('#data')?.getAttribute('src')).toBe(image)
    expect(output.querySelector('#remote')?.hasAttribute('src')).toBe(false)
    expect(output.querySelector('#remote')?.hasAttribute('srcset')).toBe(false)
    expect(output.querySelector('#relative')?.hasAttribute('src')).toBe(false)
    expect(output.querySelector('use')?.getAttribute('xlink:href')).toBe('#wheel')
    expect(output.querySelector('#embedded')?.getAttribute('href')).toBe(image)
    expect(output.querySelector('#external')?.hasAttribute('href')).toBe(false)
  })

  it('also removes external resources inside inert templates', () => {
    const output = parse('<template shadowrootmode="open"><iframe src="https://example.invalid"></iframe><template><script src="https://example.invalid/script.js"></script><img src="https://example.invalid/image.png"></template><svg viewBox="0 0 10 10"/></template>')
    const template = output.querySelector('template')!
    expect(template.hasAttribute('shadowrootmode')).toBe(false)
    expect(template.content.querySelector('iframe')).toBeNull()
    const nested = template.content.querySelector('template')!
    expect(nested.content.querySelector('script')).toBeNull()
    expect(nested.content.querySelector('img')?.hasAttribute('src')).toBe(false)
    expect(template.content.querySelector('svg')).not.toBeNull()
  })

  it('never attaches model elements or runs inline scripts in the application', () => {
    createModelEvaluationDocument('<div id="model-document-only">preview</div><script>document.body.dataset.modelExecuted = "yes"</script>')
    expect(document.querySelector('#model-document-only')).toBeNull()
    expect(document.body.dataset.modelExecuted).toBeUndefined()
  })
})
