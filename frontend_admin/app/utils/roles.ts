/**
 * 角色 / 状态常量。
 *
 * ⚠️ 数值必须与后端**逐字一致**：
 *   - `backend/internal/user/domain/user.go`  的 `Role` / `Status`
 *   - `RoleAdmin = 1` / `RoleUser = 2` / `RoleGuest = 3`
 *   - `StatusActive = 1` / `StatusDisabled = 2` / `StatusBanned = 3`
 *
 * 这条注释不是装饰。这里曾写反过：管理端把 1 当"普通用户"、2 当"管理员"，
 * 而后端相反 —— 于是在编辑弹窗里选「普通用户」提交的是 `role: 1`，
 * 后端据此把该用户**设为管理员**。这是一条提权方向的错误，而且当时列表里
 * 看不到角色（后端没返回 role），所以从界面上完全察觉不到。
 *
 * 为什么不用契约里的枚举：`role` / `status` 在契约里是 integer（swag 的
 * `swaggertype`），没有暴露枚举值；契约只能保证"是个数字"。
 * 所以这层映射必须手写，并由 `test/unit/roles.test.ts` 钉住数值。
 *
 * 受限语义（[ADR-0017](../../../docs/adr/0017-publish-display-switches-and-hot-top-operator-slot.md)
 * 决策 3）：只有管理员与注册用户算"登录"，访客（3）与库里未入枚举的 `role=0`
 * 在鉴权层视同未登录 —— 能浏览公开内容，但任何写操作与"自己的"接口都不被承认。
 * 因此这两个角色的标签必须读出"受限"，**不能显示成「注册用户」**：
 * 否则管理员会把一个写不了任何东西的账号当成正常账号。
 */

/** 管理员。 */
export const ROLE_ADMIN = 1
/** 注册用户（非管理员中唯一能写入的角色）。 */
export const ROLE_USER = 2
/** 受限访客：能浏览公开内容，不能创建 / 发布 / 互动（鉴权层视同未登录）。 */
export const ROLE_GUEST = 3
/** 未知角色：库里未入枚举的 `role=0`（列默认值），同样按受限访客处理。 */
export const ROLE_UNKNOWN = 0

/** 正常。 */
export const STATUS_ACTIVE = 1
/** 被管理员禁用（可解禁）。 */
export const STATUS_DISABLED = 2
/** 封禁。 */
export const STATUS_BANNED = 3

const ROLE_UNKNOWN_LABEL = '未知角色（按受限访客处理）'

const ROLE_LABELS: Record<number, string> = {
  [ROLE_ADMIN]: '管理员',
  [ROLE_USER]: '注册用户',
  [ROLE_GUEST]: '受限访客（无法写入）',
  [ROLE_UNKNOWN]: ROLE_UNKNOWN_LABEL,
}

const STATUS_LABELS: Record<number, string> = {
  [STATUS_ACTIVE]: '正常',
  [STATUS_DISABLED]: '禁用',
  [STATUS_BANNED]: '封禁',
}

/** 取角色文案；未知数值按未知角色（最小权限）。 */
function roleText(value: number): string {
  return ROLE_LABELS[value] ?? ROLE_UNKNOWN_LABEL
}

/** 角色展示文案。缺省或未知值按受限访客处理（最小权限）。 */
export function roleLabel(value?: number): string {
  return roleText(value ?? ROLE_UNKNOWN)
}

/** 状态展示文案。未知值按正常展示。 */
export function statusLabel(value?: number): string {
  return STATUS_LABELS[value ?? STATUS_ACTIVE] ?? STATUS_LABELS[STATUS_ACTIVE]!
}

/** 角色下拉选项（顺序按权限从高到低）。访客保留，但标注受限。 */
export const roleOptions = [
  { label: roleText(ROLE_ADMIN), value: ROLE_ADMIN },
  { label: roleText(ROLE_USER), value: ROLE_USER },
  { label: roleText(ROLE_GUEST), value: ROLE_GUEST },
]

/** 状态下拉选项。 */
export const statusOptions = [
  { label: '正常', value: STATUS_ACTIVE },
  { label: '禁用', value: STATUS_DISABLED },
  { label: '封禁', value: STATUS_BANNED },
]
