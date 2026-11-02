/**
 * 轮播标题的纯文本收敛。
 *
 * 标题在库里就是一段纯文本（后端只去首尾空白，不解析 HTML / Markdown），
 * 读者站因此也只做两件事：缺失当空串、去掉首尾空白。**不要**在渲染路径上
 * 引入 markdown-it / DOMPurify：契约里没有富文本，解析只会多出一个清洗面。
 */
export function bannerTitleText(raw?: string | null): string {
  if (typeof raw !== "string") return "";
  return raw.trim();
}
