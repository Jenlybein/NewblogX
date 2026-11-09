<script setup lang="ts">
/**
 * 富文本卡正文编辑器。
 *
 * 只提供基础排版入口（标题 / 粗体 / 斜体 / 列表 / 链接），不提供脚本、iframe 或
 * 内联事件控件：操作者无法从工具栏制造可执行内容。正文按 HTML 片段随整表保存，
 * 落库前与读者站渲染前都会过同一份白名单。
 *
 * 用可编辑区而不是纯文本域：运营看到的就是读者看到的效果，标签不必手写。
 *
 * 「链接」必须问地址：早先这里用 `execCommand('createLink', …, 'https://example.com')`
 * 写死了一个占位地址，而正文是 contenteditable —— 没有源码视图可以让运营把
 * 那个占位改掉，于是**每一条链接都真的指向 example.com**。现在改成选中文字后
 * 弹一个地址输入条，按运营填的地址建链接；如果选中的已经是既有链接，就改它的
 * 地址而不是再套一层。
 */
import { nextTick, onMounted, ref, watch } from 'vue'

import { normalizeRichTextLink } from '~/utils/richTextLink'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

/** 工具栏只列这几类排版，顺序即界面上从语义到行内的顺序。 */
const TOOLS = [
  { format: 'h2', label: '标题', title: '小标题' },
  { format: 'bold', label: 'B', title: '粗体' },
  { format: 'italic', label: 'I', title: '斜体' },
  { format: 'list', label: '•', title: '无序列表' },
  { format: 'link', label: '链接', title: '给选中的文字加链接' },
] as const

const bodyRef = ref<HTMLElement | null>(null)
// 最近一次由编辑器自身写出的值：用它避免"输入 → 回灌 innerHTML"把光标弹到开头。
let lastEmitted = ''

/** 链接输入条的状态：面板是否展开、运营填的地址、以及就地报错。 */
const linkPanelOpen = ref(false)
const linkDraft = ref('')
const linkError = ref('')
const linkInputRef = ref<HTMLInputElement | null>(null)

/**
 * 打开面板时冻结的选区。
 *
 * 必须冻住：面板一出现焦点就离开可编辑区，浏览器会把选区留在原地、但范围对象
 * 可能已经失效，等运营点「确定」时再去取就已经晚了。
 */
let savedRange: Range | null = null
/** 选中的位置已经在某个链接里：这次是改地址，不是新建链接。 */
let savedAnchor: HTMLAnchorElement | null = null

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
 * 「链接」不走 execCommand：它要的是运营填的地址，见 {@link openLinkPanel}。
 */
function apply(format: (typeof TOOLS)[number]['format']) {
  const target = bodyRef.value
  if (!target) return

  if (format === 'link') {
    openLinkPanel()
    return
  }

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
  }
  emitContent()
}

/** 从节点往上找到可编辑区里的 `<a>`（选中文字落在既有链接上时用）。 */
function closestAnchor(node: Node, root: HTMLElement): HTMLAnchorElement | null {
  let current: Node | null = node
  while (current && current !== root) {
    if (current instanceof Element && current.tagName === 'A') {
      return current as HTMLAnchorElement
    }
    current = current.parentNode
  }
  return null
}

/** 记下当前选区（必须仍在可编辑区内），没有有效选区时返回 null。 */
function captureSelection(): { range: Range; anchor: HTMLAnchorElement | null } | null {
  const body = bodyRef.value
  if (!body || typeof window === 'undefined') return null
  const selection = window.getSelection()
  if (!selection || selection.rangeCount === 0) return null
  const range = selection.getRangeAt(0)
  if (!body.contains(range.commonAncestorContainer)) return null
  return { range: range.cloneRange(), anchor: closestAnchor(range.commonAncestorContainer, body) }
}

/** 打开链接面板；选区落在既有链接上时把原地址回填进输入框。 */
function openLinkPanel() {
  if (linkPanelOpen.value) {
    cancelLink()
    return
  }
  const captured = captureSelection()
  savedRange = captured?.range ?? null
  savedAnchor = captured?.anchor ?? null
  linkDraft.value = savedAnchor?.getAttribute('href') ?? ''
  linkError.value = ''
  linkPanelOpen.value = true
  void nextTick(() => linkInputRef.value?.focus())
}

function cancelLink() {
  linkPanelOpen.value = false
  linkError.value = ''
  savedRange = null
  savedAnchor = null
}

/** 把光标挪到某个节点之后（链接建完继续输入时不会跑到正文开头）。 */
function moveCaretAfter(node: Node, root: HTMLElement) {
  if (typeof window === 'undefined') return
  const selection = window.getSelection()
  if (!selection) return
  const after = document.createRange()
  after.setStartAfter(node)
  after.collapse(true)
  selection.removeAllRanges()
  selection.addRange(after)
  root.focus()
}

/** 在冻结的选区上建一条链接（选区里有块级内容时退化成插入地址文本）。 */
function insertLink(url: string) {
  const body = bodyRef.value
  if (!body) return

  const range = savedRange
  // 没有捕获到选区（例如运营先点了面板才回来点确定）：把链接追加到正文末尾。
  if (!range) {
    const anchor = document.createElement('a')
    anchor.setAttribute('href', url)
    anchor.textContent = url
    body.appendChild(anchor)
    moveCaretAfter(anchor, body)
    return
  }

  // 选中的是块级内容（整段 / 整行）时不要把 `<a>` 套在外面：那在读者站
  // 会被白名单按内联元素处理，段落结构会变得不可预期。退化成插入地址本身。
  const fragment = range.collapsed ? null : range.extractContents()
  const hasBlock = fragment
    ? fragment.querySelector('h2,p,div,ul,ol,li,blockquote,pre') !== null
    : false

  if (!fragment || hasBlock || range.collapsed) {
    if (fragment) {
      // 取出来的块级内容原样放回去，不改结构。
      range.insertNode(fragment)
    }
    const anchor = document.createElement('a')
    anchor.setAttribute('href', url)
    anchor.textContent = url
    range.collapse(true)
    range.insertNode(anchor)
    moveCaretAfter(anchor, body)
    return
  }

  const anchor = document.createElement('a')
  anchor.setAttribute('href', url)
  anchor.appendChild(fragment)
  range.insertNode(anchor)
  moveCaretAfter(anchor, body)
}

/** 确定：地址合法才落进正文，不合法就地报错、面板不关。 */
function confirmLink() {
  const result = normalizeRichTextLink(linkDraft.value)
  if (!result.ok) {
    linkError.value = result.error
    return
  }

  if (savedAnchor) {
    // 选中的已经是既有链接：改地址，不新套一层（套一层会留下一个空 `<a>`）。
    savedAnchor.setAttribute('href', result.url)
    const body = bodyRef.value
    if (body) moveCaretAfter(savedAnchor, body)
  } else {
    insertLink(result.url)
  }

  emitContent()
  cancelLink()
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
        @mousedown.prevent
        @click="apply(tool.format)"
      >{{ tool.label }}</button>
    </div>

    <div v-if="linkPanelOpen" class="sidebar-richtext__link" data-role="rich-text-link-panel">
      <input
        ref="linkInputRef"
        v-model="linkDraft"
        type="text"
        class="sidebar-richtext__link-input"
        data-role="rich-text-link-input"
        aria-label="链接地址"
        placeholder="填写链接，例如 https://example.cn/post 或 /article/123"
        @keydown.enter.prevent="confirmLink"
        @keydown.esc.prevent="cancelLink">
      <button type="button" class="sidebar-richtext__link-btn" data-role="rich-text-link-confirm" @click="confirmLink">确定</button>
      <button type="button" class="sidebar-richtext__link-btn" data-role="rich-text-link-cancel" @click="cancelLink">取消</button>
      <p v-if="linkError" class="sidebar-richtext__link-error" data-role="rich-text-link-error">{{ linkError }}</p>
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
    <p class="admin-hint">支持标题、粗体、斜体、列表与链接；选中文字后点「链接」填地址，选中已有链接则改它的地址。脚本、iframe 与内联事件不会进入读者站。</p>
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

/* 链接地址输入条：夹在工具栏与正文之间，不弹模态，运营填完就走。 */
.sidebar-richtext__link {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.375rem;
  border: 1px solid var(--color-line);
  border-bottom: 0;
  background: var(--color-bg-subtle);
  padding: 0.375rem;
}

.sidebar-richtext__link-input {
  min-width: 0;
  flex: 1 1 12rem;
  border: 1px solid var(--color-line-strong);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  padding: 0.25rem 0.5rem;
  color: var(--color-ink);
  font-size: 0.8125rem;
}

.sidebar-richtext__link-btn {
  border: 1px solid var(--color-line-strong);
  border-radius: var(--radius-sm);
  padding: 0.25rem 0.75rem;
  color: var(--color-ink);
  font-size: 0.8125rem;
  line-height: 1.4;
}

.sidebar-richtext__link-error {
  flex-basis: 100%;
  margin: 0;
  /* 用"危险文字"档而不是危险填充档：这里要先读得清。 */
  color: var(--color-danger-ink);
  font-size: 0.75rem;
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
