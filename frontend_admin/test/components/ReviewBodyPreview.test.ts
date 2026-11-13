import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ReviewBodyPreview from '~/components/review/ReviewBodyPreview.vue'

/**
 * 审核列表的正文单元格（2026-11-13 改版）。
 *
 * 旧版是"短稿直接展示 + 长稿预览/展开"，正文随列表一起下发。现在列表**不带正文**，
 * 单元格只留一个「查看正文」按钮，点开才按需拉一次，弹窗里用 Markdown 渲染。
 *
 * 这里钉三件事：
 *   1. 单元格本身不展示正文（也不发请求）—— 列表不该因为要显示正文而变重；
 *   2. 点开才请求，且同一行重复点开不重复请求；
 *   3. 渲染走 markdown-it 且 `html: false`：作者提交的内联 HTML 必须被转义，
 *      不能变成后台里的可执行脚本。
 *
 * 弹窗内容用 NModal 渲染，它会 teleport 到 body —— 所以断言走 `document`，
 * 不是 wrapper（wrapper 里只剩 teleport 标记）。
 */

const { getReviewTaskContent } = vi.hoisted(() => ({ getReviewTaskContent: vi.fn() }))

vi.mock('~/services/admin', () => ({ getReviewTaskContent }))

function mountBody(props: { taskId?: string; articleTitle?: string }) {
  return mount(ReviewBodyPreview, { props, attachTo: document.body })
}

function query(selector: string): HTMLElement | null {
  return document.body.querySelector(selector)
}

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('ReviewBodyPreview：按需拉取 + Markdown 渲染', () => {
  it('单元格只给按钮，不请求、不展示正文', () => {
    const wrapper = mountBody({ taskId: 't1', articleTitle: '待审稿' })

    expect(wrapper.find('[data-role="review-body-open"]').exists()).toBe(true)
    expect(query('[data-role="review-body-content"]')).toBeNull()
    expect(getReviewTaskContent).not.toHaveBeenCalled()
  })

  it('点开才拉正文，并用 Markdown 渲染', async () => {
    getReviewTaskContent.mockResolvedValue({ content: '# 标题\n\n**加粗**正文' })
    const wrapper = mountBody({ taskId: 't1', articleTitle: '待审稿' })

    await wrapper.get('[data-role="review-body-open"]').trigger('click')
    await flushPromises()

    expect(getReviewTaskContent).toHaveBeenCalledWith('t1')
    const rendered = query('[data-role="review-body-content"]')
    expect(rendered).not.toBeNull()
    expect(rendered!.innerHTML).toContain('<h1>标题</h1>')
    expect(rendered!.innerHTML).toContain('<strong>加粗</strong>')
    expect(rendered!.textContent).not.toContain('**加粗**')
  })

  it('内联 HTML 被转义（作者内容不能变成后台里的脚本）', async () => {
    getReviewTaskContent.mockResolvedValue({ content: '<img src=x onerror="alert(1)">' })
    const wrapper = mountBody({ taskId: 't1' })

    await wrapper.get('[data-role="review-body-open"]').trigger('click')
    await flushPromises()

    const rendered = query('[data-role="review-body-content"]')
    expect(rendered).not.toBeNull()
    // 关键不是"文档里看不到 onerror 这个词"，而是它**只以转义文本**出现：
    // 没有真正的 <img> 元素，标签本身被转成 &lt;img。
    expect(rendered!.querySelector('img')).toBeNull()
    expect(rendered!.innerHTML).toContain('&lt;img')
  })

  it('重复点开不重复请求', async () => {
    getReviewTaskContent.mockResolvedValue({ content: '正文' })
    const wrapper = mountBody({ taskId: 't1' })

    await wrapper.get('[data-role="review-body-open"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-role="review-body-open"]').trigger('click')
    await flushPromises()

    expect(getReviewTaskContent).toHaveBeenCalledTimes(1)
  })

  it('正文为空（文章已删除）给出可处理提示，而不是空白弹窗', async () => {
    getReviewTaskContent.mockResolvedValue({ content: '' })
    const wrapper = mountBody({ taskId: 't1' })

    await wrapper.get('[data-role="review-body-open"]').trigger('click')
    await flushPromises()

    expect(query('[data-role="review-body-empty"]')?.textContent).toContain('正文不可用')
  })

  it('请求失败时给出错误提示，不把弹窗留空', async () => {
    getReviewTaskContent.mockRejectedValue(new Error('网络断了'))
    const wrapper = mountBody({ taskId: 't1' })

    await wrapper.get('[data-role="review-body-open"]').trigger('click')
    await flushPromises()

    expect(query('[data-role="review-body-error"]')?.textContent).toContain('正文加载失败')
  })
})
