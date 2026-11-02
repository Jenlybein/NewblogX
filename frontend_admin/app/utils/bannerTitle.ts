/**
 * 后台轮播标题的纯文本收敛。
 *
 * 标题在库里就是一段纯文本（后端只去首尾空白，不解析 HTML / Markdown），
 * 后台也只做"去首尾空白 + 空标题给占位"这两件事。**不要**在这里做
 * HTML 白名单 / Markdown 解析：契约里没有富文本，多一层解析就多一个清洗面。
 */

/** 取出可提交/可展示的标题：非字符串与纯空白一律当没填。 */
export function bannerTitleText(raw?: string | null): string {
  if (typeof raw !== 'string') return ''
  return raw.trim()
}

/** 列表单元格要显示的文字：没标题时给一个可读占位，而不是一片空白。 */
export function bannerTitleCell(raw?: string | null): string {
  return bannerTitleText(raw) || '未填写'
}
