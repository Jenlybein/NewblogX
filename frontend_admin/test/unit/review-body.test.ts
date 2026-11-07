import { describe, expect, it } from 'vitest'

import {
  REVIEW_BODY_PREVIEW_LIMIT,
  isReviewBodyLong,
  reviewBodyPlainText,
  reviewBodyPreview,
} from '~/utils/reviewBody'

// 审核队列正文预览的纯逻辑。审核员看到的是**可读文本**，不是 HTML 源码：
// 列表里直接判断稿子，标签本身只添乱；而且正文由作者提供，后台不渲染它。
describe('reviewBodyPlainText', () => {
  it('空内容返回空串', () => {
    expect(reviewBodyPlainText(undefined)).toBe('')
    expect(reviewBodyPlainText(null)).toBe('')
    expect(reviewBodyPlainText('')).toBe('')
  })

  it('块级标签换成换行，行内标签直接去掉', () => {
    expect(reviewBodyPlainText('<p>第一段</p><p>第二段</p>')).toBe('第一段\n第二段')
    expect(reviewBodyPlainText('前<b>粗</b>后')).toBe('前粗后')
    expect(reviewBodyPlainText('一行<br>另一行')).toBe('一行\n另一行')
  })

  it('常见实体还原成字符', () => {
    expect(reviewBodyPlainText('a &amp; b &lt;c&gt; &quot;d&quot;&nbsp;e')).toBe('a & b <c> "d" e')
  })

  it('多余空行折叠，首尾空白去掉', () => {
    expect(reviewBodyPlainText('  <p>甲</p>\n\n\n<p>乙</p>  ')).toBe('甲\n乙')
  })
})

describe('isReviewBodyLong', () => {
  it('按纯文本字符数判断，边界值不算长', () => {
    expect(isReviewBodyLong('长'.repeat(REVIEW_BODY_PREVIEW_LIMIT))).toBe(false)
    expect(isReviewBodyLong('长'.repeat(REVIEW_BODY_PREVIEW_LIMIT + 1))).toBe(true)
  })

  it('标签不计入长度', () => {
    expect(isReviewBodyLong(`<p>${'短'.repeat(20)}</p><p></p><p></p>`)).toBe(false)
  })
})

describe('reviewBodyPreview', () => {
  it('短内容原样返回', () => {
    expect(reviewBodyPreview('<p>短正文</p>')).toBe('短正文')
  })

  it('长内容截到上限并带省略号', () => {
    const long = '长'.repeat(REVIEW_BODY_PREVIEW_LIMIT + 10)
    const out = reviewBodyPreview(long)
    expect(out.endsWith('…')).toBe(true)
    expect(out.slice(0, -1)).toBe('长'.repeat(REVIEW_BODY_PREVIEW_LIMIT))
  })
})
