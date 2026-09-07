<template>
  <Teleport to="body">
    <Transition name="modal-fade">
      <div v-if="visible && hasContent" class="modal-overlay" @click="handleOverlayClick">
        <div class="modal-container" role="dialog" aria-modal="true" :aria-label="modalTitle">
          <!-- 装饰光晕 -->
          <div class="modal-glow" aria-hidden="true"></div>

          <!-- 模态框头部 -->
          <div class="modal-header">
            <div class="header-left">
              <div class="header-icon" aria-hidden="true">
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <path d="M3 11h3a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-5Zm0 0a9 9 0 1 1 18 0m0 0v5a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3Z" />
                </svg>
              </div>
              <div class="header-text">
                <h2 class="modal-title">{{ modalTitle }}</h2>
                <p class="modal-subtitle">{{ subtitle }}</p>
              </div>
            </div>
            <button
              type="button"
              class="close-btn"
              :aria-label="t('common.close')"
              :title="t('common.close')"
              @click="closeModal"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>

          <!-- 模态框内容 -->
          <div class="modal-body">
            <div
              v-if="sanitizedContent"
              class="markdown-content"
              data-testid="customer-support-content"
              v-html="sanitizedContent"
            ></div>
            <div v-else class="empty-state">
              <p>{{ t('common.customerSupportEmpty') }}</p>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores'
import { createBodyScrollLock } from '@/utils/bodyScrollLock'

interface Props {
  visible: boolean
}

interface Emits {
  (e: 'update:visible', value: boolean): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t } = useI18n()
const appStore = useAppStore()
const bodyScrollLock = createBodyScrollLock()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name?.trim() || '')

const modalTitle = computed(() => {
  return siteName.value ? `${siteName.value} · ${t('common.contactSupport')}` : t('common.contactSupport')
})

const subtitle = computed(() => t('common.customerSupportSubtitle'))

const rawContent = computed(() => appStore.cachedPublicSettings?.customer_support_content || '')
const hasContent = computed(() => rawContent.value.trim().length > 0)

marked.setOptions({ breaks: true, gfm: true })

const sanitizedContent = computed(() => {
  if (!hasContent.value) return ''

  try {
    const htmlContent = marked.parse(rawContent.value) as string
    const sanitized = DOMPurify.sanitize(htmlContent, {
      ALLOWED_TAGS: [
        'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
        'p', 'br', 'hr',
        'ul', 'ol', 'li',
        'strong', 'em', 'u', 's', 'del', 'code', 'pre',
        'a', 'img', 'blockquote',
        'table', 'thead', 'tbody', 'tr', 'th', 'td',
      ],
      ALLOWED_ATTR: ['href', 'src', 'alt', 'title', 'target', 'rel', 'class'],
      ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|tel):|data:image\/(?:png|gif|jpe?g|webp);|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i,
      ALLOW_DATA_ATTR: false,
    })

    if (typeof document === 'undefined') return sanitized
    const template = document.createElement('template')
    template.innerHTML = sanitized
    template.content.querySelectorAll('img[src]').forEach((image) => {
      const src = image.getAttribute('src')?.trim() || ''
      if (/^data:/i.test(src) && !/^data:image\/(?:png|gif|jpe?g|webp);/i.test(src)) {
        image.removeAttribute('src')
      }
    })
    template.content.querySelectorAll('a[target="_blank"]').forEach((anchor) => {
      const rel = new Set((anchor.getAttribute('rel') || '').split(/\s+/).filter(Boolean))
      rel.add('noopener')
      rel.add('noreferrer')
      anchor.setAttribute('rel', Array.from(rel).join(' '))
    })
    template.content.querySelectorAll('a[href]').forEach((anchor) => {
      if (/^data:/i.test(anchor.getAttribute('href')?.trim() || '')) anchor.removeAttribute('href')
    })
    return template.innerHTML
  } catch {
    return ''
  }
})

function closeModal() {
  emit('update:visible', false)
}

function handleOverlayClick(event: MouseEvent) {
  if (event.target === event.currentTarget) closeModal()
}

function handleEscapeKey(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.visible) closeModal()
}

watch(
  () => [props.visible, hasContent.value] as const,
  ([visible, content]) => {
    if (!visible || !content) {
      if (props.visible && !content) closeModal()
      bodyScrollLock.set(false)
    } else {
      bodyScrollLock.set(true)
    }
  },
  { immediate: true },
)

onMounted(() => {
  window.addEventListener('keydown', handleEscapeKey)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleEscapeKey)
  bodyScrollLock.release()
})
</script>

<style scoped>
/* 模态框过渡动画 */
.modal-fade-enter-active,
.modal-fade-leave-active {
  transition: opacity 0.25s ease;
}

.modal-fade-enter-active .modal-container,
.modal-fade-leave-active .modal-container {
  transition: transform 0.32s cubic-bezier(0.34, 1.56, 0.64, 1), opacity 0.25s ease;
}

.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}

.modal-fade-enter-from .modal-container,
.modal-fade-leave-to .modal-container {
  transform: translateY(16px) scale(0.96);
  opacity: 0;
}

/* 模态框遮罩层 */
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 10000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: rgba(15, 23, 42, 0.55);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

/* 模态框容器 */
.modal-container {
  position: relative;
  width: 100%;
  max-width: 480px;
  max-height: 84vh;
  background: #ffffff;
  border: 1px solid rgba(255, 255, 255, 0.6);
  border-radius: 20px;
  box-shadow:
    0 24px 70px -12px rgba(15, 23, 42, 0.35),
    0 8px 24px -8px rgba(15, 23, 42, 0.2);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

html.dark .modal-container {
  background: #0f172a;
  border-color: rgba(148, 163, 184, 0.16);
  box-shadow:
    0 24px 70px -12px rgba(0, 0, 0, 0.7),
    0 8px 24px -8px rgba(0, 0, 0, 0.5);
}

/* 顶部装饰光晕 */
.modal-glow {
  position: absolute;
  top: -120px;
  left: 50%;
  width: 320px;
  height: 220px;
  transform: translateX(-50%);
  background: radial-gradient(
    circle at center,
    rgba(20, 184, 166, 0.35) 0%,
    rgba(14, 165, 233, 0.18) 45%,
    transparent 72%
  );
  pointer-events: none;
  z-index: 0;
}

/* 模态框头部 */
.modal-header {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 22px 22px 18px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.8);
}

html.dark .modal-header {
  border-bottom-color: rgba(51, 65, 85, 0.6);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.header-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 44px;
  height: 44px;
  color: #ffffff;
  background: linear-gradient(135deg, #14b8a6 0%, #0ea5e9 100%);
  border-radius: 14px;
  box-shadow: 0 6px 16px -4px rgba(14, 165, 233, 0.5);
}

.header-icon svg {
  width: 24px;
  height: 24px;
}

.header-text {
  min-width: 0;
}

.modal-title {
  margin: 0;
  color: #0f172a;
  font-size: 17px;
  font-weight: 700;
  line-height: 1.35;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

html.dark .modal-title {
  color: #f1f5f9;
}

.modal-subtitle {
  margin: 3px 0 0;
  color: #94a3b8;
  font-size: 12.5px;
  line-height: 1.4;
}

html.dark .modal-subtitle {
  color: #64748b;
}

.close-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 34px;
  height: 34px;
  padding: 0;
  color: #64748b;
  background: rgba(241, 245, 249, 0.7);
  border: none;
  border-radius: 10px;
  cursor: pointer;
  transition: color 0.2s ease, background 0.2s ease, transform 0.2s ease;
}

.close-btn:hover {
  color: #0f172a;
  background: #e2e8f0;
  transform: rotate(90deg);
}

html.dark .close-btn {
  color: #94a3b8;
  background: rgba(51, 65, 85, 0.5);
}

html.dark .close-btn:hover {
  color: #f1f5f9;
  background: #334155;
}

.close-btn svg {
  width: 18px;
  height: 18px;
}

/* 模态框内容区域 */
.modal-body {
  position: relative;
  z-index: 1;
  flex: 1;
  padding: 22px;
  overflow-y: auto;
}

/* 自定义滚动条 */
.modal-body::-webkit-scrollbar {
  width: 8px;
}

.modal-body::-webkit-scrollbar-track {
  background: transparent;
}

.modal-body::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border: 2px solid transparent;
  background-clip: padding-box;
  border-radius: 8px;
}

.modal-body::-webkit-scrollbar-thumb:hover {
  background: #94a3b8;
  background-clip: padding-box;
}

html.dark .modal-body::-webkit-scrollbar-thumb {
  background: #475569;
  background-clip: padding-box;
}

html.dark .modal-body::-webkit-scrollbar-thumb:hover {
  background: #64748b;
  background-clip: padding-box;
}

/* 空状态 */
.empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 180px;
  color: #9ca3af;
  text-align: center;
}

html.dark .empty-state {
  color: #64748b;
}

/* Markdown 内容样式 */
.markdown-content {
  color: #334155;
  font-size: 15px;
  line-height: 1.75;
}

html.dark .markdown-content {
  color: #cbd5e1;
}

.markdown-content :deep(h1),
.markdown-content :deep(h2),
.markdown-content :deep(h3),
.markdown-content :deep(h4),
.markdown-content :deep(h5),
.markdown-content :deep(h6) {
  margin: 22px 0 14px;
  color: #0f172a;
  font-weight: 700;
  line-height: 1.3;
}

html.dark .markdown-content :deep(h1),
html.dark .markdown-content :deep(h2),
html.dark .markdown-content :deep(h3),
html.dark .markdown-content :deep(h4),
html.dark .markdown-content :deep(h5),
html.dark .markdown-content :deep(h6) {
  color: #f1f5f9;
}

.markdown-content :deep(h1) {
  font-size: 22px;
}

.markdown-content :deep(h2) {
  font-size: 19px;
}

.markdown-content :deep(h3) {
  font-size: 17px;
}

.markdown-content :deep(h1:first-child),
.markdown-content :deep(h2:first-child),
.markdown-content :deep(h3:first-child),
.markdown-content :deep(p:first-child) {
  margin-top: 0;
}

.markdown-content :deep(p) {
  margin: 12px 0;
}

.markdown-content :deep(a) {
  color: #0ea5e9;
  font-weight: 500;
  text-decoration: none;
  transition: color 0.2s ease;
}

.markdown-content :deep(a:hover) {
  color: #0284c7;
  text-decoration: underline;
}

html.dark .markdown-content :deep(a) {
  color: #38bdf8;
}

html.dark .markdown-content :deep(a:hover) {
  color: #7dd3fc;
}

.markdown-content :deep(img) {
  display: block;
  max-width: 220px;
  width: 100%;
  height: auto;
  margin: 16px auto;
  padding: 8px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 14px;
  box-shadow: 0 8px 24px -8px rgba(15, 23, 42, 0.18);
}

html.dark .markdown-content :deep(img) {
  background: #f8fafc;
  border-color: #1e293b;
}

.markdown-content :deep(code) {
  padding: 2px 6px;
  color: #0d9488;
  font-family: 'SF Mono', Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  font-size: 0.88em;
  background: rgba(20, 184, 166, 0.1);
  border-radius: 6px;
}

html.dark .markdown-content :deep(code) {
  color: #5eead4;
  background: rgba(20, 184, 166, 0.16);
}

.markdown-content :deep(pre) {
  padding: 16px;
  margin: 16px 0;
  overflow-x: auto;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
}

html.dark .markdown-content :deep(pre) {
  background: #1e293b;
  border-color: #334155;
}

.markdown-content :deep(pre code) {
  padding: 0;
  color: inherit;
  background: transparent;
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  margin: 12px 0;
  padding-left: 22px;
}

.markdown-content :deep(li) {
  margin: 6px 0;
}

.markdown-content :deep(blockquote) {
  margin: 16px 0;
  padding: 8px 16px;
  color: #475569;
  background: rgba(20, 184, 166, 0.06);
  border-left: 3px solid #14b8a6;
  border-radius: 0 8px 8px 0;
}

html.dark .markdown-content :deep(blockquote) {
  color: #94a3b8;
  background: rgba(20, 184, 166, 0.1);
  border-left-color: #14b8a6;
}

.markdown-content :deep(hr) {
  margin: 22px 0;
  border: none;
  border-top: 1px solid #e2e8f0;
}

html.dark .markdown-content :deep(hr) {
  border-top-color: #334155;
}

.markdown-content :deep(table) {
  width: 100%;
  margin: 16px 0;
  border-collapse: collapse;
  border-radius: 10px;
  overflow: hidden;
}

.markdown-content :deep(th),
.markdown-content :deep(td) {
  padding: 9px 12px;
  text-align: left;
  border: 1px solid #e2e8f0;
}

html.dark .markdown-content :deep(th),
html.dark .markdown-content :deep(td) {
  border-color: #334155;
}

.markdown-content :deep(th) {
  font-weight: 600;
  background: #f8fafc;
}

html.dark .markdown-content :deep(th) {
  background: #1e293b;
}

/* 响应式设计 */
@media (max-width: 768px) {
  .modal-overlay {
    align-items: flex-end;
    padding: 0;
  }

  .modal-container {
    max-width: 100%;
    max-height: 90vh;
    border-radius: 20px 20px 0 0;
  }

  /* 移动端从底部滑入 */
  .modal-fade-enter-from .modal-container,
  .modal-fade-leave-to .modal-container {
    transform: translateY(100%);
  }

  .modal-header {
    padding: 18px 18px 16px;
  }

  .modal-title {
    font-size: 16px;
  }

  .modal-body {
    padding: 18px;
  }

  .markdown-content {
    font-size: 14px;
  }
}
</style>
