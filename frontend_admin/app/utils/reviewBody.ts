/**
 * 审核队列条目的正文预览（票 307）。
 *
 * 列表里要让审核员**不跳文章页就能判断**，所以每条要带正文。后台不渲染
 * 作者正文里的 HTML：内容来自作者、不受信，而且审核员要读的是文字不是标签。
 * 这里把它折成纯文本再按字符数截断，展开/收起只切换展示长度。
 */

/** 折叠态最多展示的字符数。 */
export const REVIEW_BODY_PREVIEW_LIMIT = 200

// 块级标签换行、行内标签去掉、常见实体还原：顺序有讲究——先剥标签再还原实体，
// 否则作者写下的 `&lt;p&gt;` 会被当成真标签吃掉。
const BLOCK_BREAK = /<\/(?:p|div|li|ul|ol|h[1-6]|blockquote|tr|section|article)>|<br\s*\/?>|<hr\s*\/?>/giu
const HTML_TAG = /<[^>]*>/gu
const ENTITY = /&(?:nbsp|amp|lt|gt|quot|#39|#x27);/giu
const ENTITY_TEXT: Record<string, string> = {
  '&nbsp;': ' ',
  '&amp;': '&',
  '&lt;': '<',
  '&gt;': '>',
  '&quot;': '"',
  '&#39;': "'",
  '&#x27;': "'",
}

/** 把正文 HTML 折成审核员可读的纯文本。 */
export function reviewBodyPlainText(content?: string | null): string {
  if (!content) return ''
  return content
    .replace(/\r\n?/gu, '\n')
    .replace(BLOCK_BREAK, '\n')
    .replace(HTML_TAG, '')
    .replace(ENTITY, entity => ENTITY_TEXT[entity.toLowerCase()] ?? entity)
    .replace(/[ \t]+/gu, ' ')
    .replace(/\n\s*\n+/gu, '\n')
    .split('\n')
    .map(line => line.trim())
    .join('\n')
    .trim()
}

/** 正文是否长到需要展开/收起。 */
export function isReviewBodyLong(content?: string | null): boolean {
  return reviewBodyPlainText(content).length > REVIEW_BODY_PREVIEW_LIMIT
}

/** 折叠态展示的正文：短稿原样，长稿截到上限并带省略号。 */
export function reviewBodyPreview(content?: string | null): string {
  const plain = reviewBodyPlainText(content)
  if (plain.length <= REVIEW_BODY_PREVIEW_LIMIT) return plain
  return `${Array.from(plain).slice(0, REVIEW_BODY_PREVIEW_LIMIT).join('')}…`
}
