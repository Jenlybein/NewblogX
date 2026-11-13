<script setup lang="ts">
/**
 * 审核列表的正文单元格（2026-11-13 改版）。
 *
 * 上一版是"短稿给全文、长稿给预览 + 展开全文"，而正文随列表接口一起下发 ——
 * 一页 10 条全文可能几百 KB，绝大多数时候审核员只看标题、风险与合规结论。
 *
 * 现在列表**不带正文**：这里只留一个「查看正文」按钮，点开才按需拉一次，
 * 在弹窗里用 Markdown 渲染完整正文。
 *
 * 渲染安全：`markdown-it` 用 `html: false` —— 正文是作者提交的内容，允许内联
 * HTML 等于把 XSS 装进后台。关掉之后 Markdown 语法照常渲染，HTML 标签按字面显示。
 * 不用 `v-html` 直接渲染接口返回的 HTML，也是为了同一条理由。
 */
import { NButton, NModal, NSpin } from 'naive-ui'
import { computed, ref } from 'vue'
import MarkdownIt from 'markdown-it'
import { getReviewTaskContent } from '~/services/admin'

const props = defineProps<{ taskId?: string; articleTitle?: string }>()

const md = new MarkdownIt({ html: false, linkify: true, breaks: true })

const visible = ref(false)
const loading = ref(false)
const error = ref('')
const content = ref('')

const rendered = computed(() => (content.value ? md.render(content.value) : ''))
const isEmpty = computed(() => !loading.value && !error.value && !content.value.trim())

async function open() {
  if (!props.taskId) return
  visible.value = true
  if (content.value) return // 同一行重复点开不重复请求
  loading.value = true
  error.value = ''
  try {
    const payload = await getReviewTaskContent(props.taskId)
    content.value = payload.content ?? ''
  } catch (e) {
    error.value = e instanceof Error ? `正文加载失败：${e.message}` : '正文加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="review-body">
    <NButton
      text
      size="tiny"
      type="primary"
      :disabled="!taskId"
      data-role="review-body-open"
      @click="open">
      查看正文
    </NButton>

    <NModal v-model:show="visible" preset="card" class="review-body-modal" :title="articleTitle || '正文'">
      <NSpin :show="loading">
        <p v-if="error" data-role="review-body-error" class="review-body__hint">{{ error }}</p>
        <p v-else-if="isEmpty" data-role="review-body-empty" class="review-body__hint">
          正文不可用（文章可能已被删除）。任务本身仍然可以处理。
        </p>
        <!-- eslint-disable-next-line vue/no-v-html -- markdown-it 已关闭内联 HTML，见上方注释 -->
        <div v-else class="review-body__markdown markdown-body" data-role="review-body-content" v-html="rendered" />
      </NSpin>
    </NModal>
  </div>
</template>

<style scoped>
.review-body__hint {
  margin: 0;
  color: var(--color-ink-muted);
}
.review-body__markdown {
  max-height: 62vh;
  overflow-y: auto;
  word-break: break-word;
}
/* 渲染出来的 Markdown 保留可读的行距与标题层级；不做全站排版，只求审核时看得清。 */
.review-body__markdown :deep(h1),
.review-body__markdown :deep(h2),
.review-body__markdown :deep(h3) {
  margin: 1rem 0 0.5rem;
  line-height: 1.3;
}
.review-body__markdown :deep(p),
.review-body__markdown :deep(li) {
  margin: 0.5rem 0;
  line-height: 1.7;
}
.review-body__markdown :deep(pre) {
  padding: 0.75rem;
  overflow-x: auto;
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
}
.review-body__markdown :deep(code) {
  font-family: var(--font-mono, ui-monospace, monospace);
  font-size: 0.875em;
}
.review-body__markdown :deep(img) {
  max-width: 100%;
}
</style>
