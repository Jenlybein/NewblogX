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
 * 所以这层映射必须手写，并由 `test/unit/role.test.ts` 钉住数值。
 */

/** 管理员。 */
export const ROLE_ADMIN = 1
/** 注册用户。 */
export const ROLE_USER = 2
/** 访客。 */
export const ROLE_GUEST = 3

/** 正常。 */
export const STATUS_ACTIVE = 1
/** 被管理员禁用（可解禁）。 */
export const STATUS_DISABLED = 2
/** 封禁。 */
export const STATUS_BANNED = 3

const ROLE_LABELS: Record<number, string> = {
  [ROLE_ADMIN]: '管理员',
  [ROLE_USER]: '注册用户',
  [ROLE_GUEST]: '访客',
}

const STATUS_LABELS: Record<number, string> = {
  [STATUS_ACTIVE]: '正常',
  [STATUS_DISABLED]: '禁用',
  [STATUS_BANNED]: '封禁',
}

/** 角色展示文案。未知值按注册用户展示（最小权限）。 */
export function roleLabel(value?: number): string {
  return ROLE_LABELS[value ?? ROLE_USER] ?? ROLE_LABELS[ROLE_USER]!
}

/** 状态展示文案。未知值按正常展示。 */
export function statusLabel(value?: number): string {
  return STATUS_LABELS[value ?? STATUS_ACTIVE] ?? STATUS_LABELS[STATUS_ACTIVE]!
}

/** 角色下拉选项（顺序按权限从高到低）。 */
export const roleOptions = [
  { label: '管理员', value: ROLE_ADMIN },
  { label: '注册用户', value: ROLE_USER },
  { label: '访客', value: ROLE_GUEST },
]

/** 状态下拉选项。 */
export const statusOptions = [
  { label: '正常', value: STATUS_ACTIVE },
  { label: '禁用', value: STATUS_DISABLED },
  { label: '封禁', value: STATUS_BANNED },
]
