import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import ReviewBodyPreview from '~/components/review/ReviewBodyPreview.vue'
import { REVIEW_BODY_PREVIEW_LIMIT } from '~/utils/reviewBody'

// 审核列表条目的正文单元格：短稿直接看全文，长稿先给预览、可展开看全文（#307）。
// 审核员在列表里就能判断，不必再跳到文章页。
describe('ReviewBodyPreview', () => {
  it('短正文直接展示，不出现展开按钮', () => {
    const wrapper = mount(ReviewBodyPreview, { props: { content: '<p>短正文</p>' } })

    expect(wrapper.get('[data-role="review-body-text"]').text()).toBe('短正文')
    expect(wrapper.find('[data-role="review-body-toggle"]').exists()).toBe(false)
  })

  it('长正文先折叠成预览，展开后看全文并可再收起', async () => {
    const full = '长'.repeat(REVIEW_BODY_PREVIEW_LIMIT + 20)
    const wrapper = mount(ReviewBodyPreview, { props: { content: `<p>${full}</p>` } })

    const text = wrapper.get('[data-role="review-body-text"]')
    expect(text.text()).toBe(`${'长'.repeat(REVIEW_BODY_PREVIEW_LIMIT)}…`)

    const toggle = wrapper.get('[data-role="review-body-toggle"]')
    expect(toggle.text()).toBe('展开全文')
    await toggle.trigger('click')
    expect(text.text()).toBe(full)
    expect(toggle.text()).toBe('收起')
    await toggle.trigger('click')
    expect(text.text()).toBe(`${'长'.repeat(REVIEW_BODY_PREVIEW_LIMIT)}…`)
    expect(toggle.text()).toBe('展开全文')
  })

  it('正文为空时给占位而不是空白单元格', () => {
    const wrapper = mount(ReviewBodyPreview, { props: { content: '' } })

    expect(wrapper.text()).toContain('无正文')
    expect(wrapper.find('[data-role="review-body-toggle"]').exists()).toBe(false)
  })
})
