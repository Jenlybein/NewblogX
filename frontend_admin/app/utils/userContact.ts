/**
 * 账号联系方式的一行摘要。
 *
 * 为什么单独抽出来：用户管理页要在列表单元格里显示"手机号 · 邮箱"，而这两个字段
 * 在账号体系里都是**登录主标识**（手机号登录 / 邮箱验证码登录）。管理员排查
 * "这是不是同一个人的两个账号"时看的正是这两位，所以这一行的取值规则值得有测试，
 * 而不是散在页面模板里（本仓库的覆盖率门槛只统计 `app/utils/`，页面层无 e2e）。
 *
 * 规则：
 * - 手机号优先、邮箱其次，都存在就都显示；
 * - 两个都没有时返回一句明确的"未绑定"，而不是空串 —— 空串会让"确实没绑"
 *   和"渲染坏了"在界面上长得一样。
 */
export function contactText(row: { phone?: string | null; email?: string | null }): string {
  const parts = [row.phone, row.email].filter((v): v is string => Boolean(v))
  return parts.length > 0 ? parts.join(" · ") : "未绑定手机号 / 邮箱"
}

/**
 * 详情抽屉里"登录地点"那一行的取值：IP 与地区用 · 连接，都没有时给 —。
 */
export function loginPlaceText(row: { ip?: string | null; addr?: string | null }): string {
  const parts = [row.ip, row.addr].filter((v): v is string => Boolean(v))
  return parts.length > 0 ? parts.join(" · ") : "—"
}
