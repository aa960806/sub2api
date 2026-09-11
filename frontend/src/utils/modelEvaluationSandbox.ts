/**
 * Model output is an untrusted document, never markup for the application DOM.
 * Keep inline animation code, but render the result ONLY inside an opaque-origin
 * iframe with sandbox="allow-scripts". CSP is the resource-loading boundary;
 * this pass removes static navigation and external-resource primitives as well.
 * Arbitrary inline JS can still navigate its own frame. The sandbox prevents it
 * from navigating the application or reading its DOM/storage, not self-navigation.
 */
export const MODEL_EVALUATION_CSP = [
  "default-src 'none'",
  "script-src 'unsafe-inline'",
  "style-src 'unsafe-inline'",
  'img-src data:',
  'font-src data:',
  "connect-src 'none'",
  "media-src 'none'",
  "frame-src 'none'",
  "child-src 'none'",
  "worker-src 'none'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
].join('; ')

const removedElements = new Set([
  'base', 'link', 'iframe', 'frame', 'frameset', 'object', 'embed', 'portal', 'fencedframe',
])
const removedAttributes = new Set([
  'srcdoc', 'srcset', 'imagesrcset', 'action', 'formaction', 'target', 'formtarget',
  'ping', 'download', 'manifest', 'codebase', 'archive', 'profile', 'shadowrootmode',
])
const resourceAttributes = new Set(['src', 'href', 'xlink:href', 'poster', 'background', 'data'])
const dataImage = /^data:image\/(?:png|jpeg|gif|webp|avif|svg\+xml|x-icon|bmp)(?:;|,)/i

function keepResource(element: Element, attribute: string, value: string): boolean {
  const trimmed = value.trim()
  if ((attribute === 'href' || attribute === 'xlink:href') && trimmed.startsWith('#')) return true
  const tag = element.localName.toLowerCase()
  const isImage = (tag === 'img' && attribute === 'src')
    || (tag === 'image' && (attribute === 'href' || attribute === 'xlink:href'))
    || attribute === 'poster' || attribute === 'background'
  return isImage && dataImage.test(trimmed)
}

function removeExternalResources(root: ParentNode): void {
  for (const element of root.querySelectorAll('*')) {
    const tag = element.localName.toLowerCase()
    if (removedElements.has(tag)
      || (tag === 'meta' && element.hasAttribute('http-equiv'))
      || (tag === 'script' && (element.hasAttribute('src') || element.hasAttribute('href') || element.hasAttribute('xlink:href')))) {
      element.remove()
      continue
    }
    for (const attribute of [...element.attributes]) {
      const name = attribute.name.toLowerCase()
      if (removedAttributes.has(name) || (resourceAttributes.has(name) && !keepResource(element, name, attribute.value))) {
        element.removeAttribute(attribute.name)
      }
    }
    if (tag === 'template') removeExternalResources((element as HTMLTemplateElement).content)
  }
}

export function createModelEvaluationDocument(html: string): string {
  // DOMParser creates a detached, scripting-disabled document. None of its
  // elements are inserted into the application's live document.
  const parsed = new DOMParser().parseFromString(html, 'text/html')
  removeExternalResources(parsed)

  const charset = parsed.createElement('meta')
  charset.setAttribute('charset', 'utf-8')
  const policy = parsed.createElement('meta')
  policy.setAttribute('http-equiv', 'Content-Security-Policy')
  policy.setAttribute('content', MODEL_EVALUATION_CSP)
  const referrer = parsed.createElement('meta')
  referrer.setAttribute('name', 'referrer')
  referrer.setAttribute('content', 'no-referrer')
  // Enforce policy before any model-authored styles or scripts are parsed.
  parsed.head.prepend(charset, policy, referrer)
  return `<!doctype html>\n${parsed.documentElement.outerHTML}`
}
