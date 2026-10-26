/**
 * 分页工具。
 *
 * 为什么单独抽出来：`/images` 这类 page/count 分页没有 `has_more` 字段，
 * 判断"还有没有下一页"要自己算。这段判断写在页面里就测不到（管理端的覆盖率
 * 只统计 `app/utils/**`），而它恰好出过一次事故——
 * 页面曾经读 `data.has_more`（后端 `ListData` 根本没有这个字段），
 * 结果恒为 undefined，「下一页」按钮永久禁用，媒体库永远只能看第一页。
 */

/**
 * 判断 page/count 分页是否还有下一页。
 *
 * @param page 当前页码，从 1 开始
 * @param pageSize 每页条数
 * @param total 总条数（后端 `res.OkWithList` 里的 `count`）
 */
export function hasNextPage(page: number, pageSize: number, total: number): boolean {
  // 非有限数按"没有下一页"处理：宁可少翻一页，也不要翻出空页。
  if (!Number.isFinite(page) || !Number.isFinite(pageSize) || !Number.isFinite(total)) {
    return false
  }
  if (page < 1 || pageSize < 1 || total < 1) {
    return false
  }
  return page * pageSize < total
}
