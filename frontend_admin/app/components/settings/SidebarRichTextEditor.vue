<script setup lang="ts">
/**
 * 富文本卡正文编辑器。
 *
 * 只提供基础排版入口（标题 / 粗体 / 斜体 / 列表 / 链接），不提供脚本、iframe 或
 * 内联事件控件：操作者无法从工具栏制造可执行内容。正文按 HTML 片段随整表保存，
 * 落库前与读者站渲染前都会过同一份白名单。
 *
 * 用可编辑区而不是纯文本域：运营看到的就是读者看到的效果，标签不必手写。
 */
import { nextTick, onMounted, ref, watch } from 'vue'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

/** 工具栏只列这几类排版，顺序即界面上从语义到行内的顺序。 */
const TOOLS = [
  { format: 'h2', label: '标题', title: '小标题' },
  { format: 'bold', label: 'B', title: '粗体' },
  { format: 'italic', label: 'I', title: '斜体' },
  { format: 'list', label: '•', title: '无序列表' },
  { format: 'link', label: '链接', title: '链接' },
] as const

const bodyRef = ref<HTMLElement | null>(null)
// 最近一次由编辑器自身写出的值：用它避免"输入 → 回灌 innerHTML"把光标弹到开头。
let lastEmitted = ''

/** 把外部值同步进可编辑区；只在外部值变化且与当前 DOM 不一致时写。 */
function syncFromModel() {
  const target = bodyRef.value
  if (!target) return
  if (props.modelValue === lastEmitted) return
  if (target.innerHTML === props.modelValue) return
  target.innerHTML = props.modelValue
}

onMounted(syncFromModel)
watch(
  () => props.modelValue,
  () => nextTick(syncFromModel),
)

/** 把可编辑区的当前 HTML 交回给草稿。 */
function emitContent() {
  const target = bodyRef.value
  if (!target) return
  lastEmitted = target.innerHTML
  emit('update:modelValue', lastEmitted)
}

function onInput() {
  emitContent()
}

/**
 * 应用一条排版命令。
 *
 * 用浏览器原生的 contenteditable 命令（选区在可编辑区内时才生效）；
 * 没有这些能力的宿主（例如测试环境）直接跳过，不报错。
 */
function apply(format: (typeof TOOLS)[number]['format']) {
  const target = bodyRef.value
  if (!target) return
  if (typeof document.execCommand !== 'function') return

  target.focus()
  switch (format) {
    case 'h2':
      document.execCommand('formatBlock', false, '<h2>')
      break
    case 'bold':
      document.execCommand('bold')
      break
    case 'italic':
      document.execCommand('italic')
      break
    case 'list':
      document.execCommand('insertUnorderedList')
      break
    case 'link':
      // 占位链接由运营替换成真实地址；不开放 javascript: 之类的输入面。
      document.execCommand('createLink', false, 'https://example.com')
      break
  }
  emitContent()
}
</script>

<template>
  <div class="sidebar-richtext" data-role="rich-text-editor">
    <div class="sidebar-richtext__toolbar" data-role="rich-text-toolbar" role="toolbar" aria-label="富文本排版">
      <button
        v-for="tool in TOOLS"
        :key="tool.format"
        type="button"
        class="sidebar-richtext__tool"
        :data-format="tool.format"
        :title="tool.title"
        @click="apply(tool.format)"
      >{{ tool.label }}</button>
    </div>
    <div
      ref="bodyRef"
      class="sidebar-richtext__body"
      data-role="rich-text-body"
      contenteditable="true"
      role="textbox"
      aria-multiline="true"
      aria-label="富文本正文"
      @input="onInput"
    />
    <p class="admin-hint">支持标题、粗体、斜体、列表与链接；脚本、iframe 与内联事件不会进入读者站。</p>
  </div>
</template>

<style scoped>
.sidebar-richtext {
  width: 100%;
}

.sidebar-richtext__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  border: 1px solid var(--color-line);
  border-bottom: 0;
  border-radius: var(--radius-md) var(--radius-md) 0 0;
  background: var(--color-bg-subtle);
  padding: 0.25rem;
}

.sidebar-richtext__tool {
  min-width: 2rem;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  padding: 0.125rem 0.5rem;
  color: var(--color-ink);
  font-size: 0.8125rem;
  line-height: 1.5;
}

.sidebar-richtext__tool:hover,
.sidebar-richtext__tool:focus-visible {
  border-color: var(--color-line-strong);
}

.sidebar-richtext__body {
  min-height: 7rem;
  border: 1px solid var(--color-line);
  border-radius: 0 0 var(--radius-md) var(--radius-md);
  background: var(--color-surface);
  padding: 0.5rem 0.75rem;
  color: var(--color-ink);
  font-size: 0.875rem;
  line-height: 1.75;
}

.sidebar-richtext__body :deep(h2) {
  margin: 0.75rem 0 0.5rem;
  font-size: 1rem;
  font-weight: 600;
}

.sidebar-richtext__body :deep(ul) {
  margin: 0.5rem 0;
  padding-left: 1.25rem;
  list-style: disc;
}

.sidebar-richtext__body :deep(a) {
  color: var(--color-link);
  text-decoration: underline;
}
</style>
