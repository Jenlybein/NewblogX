import { describe, expect, it } from 'vitest'

import {
  ROLE_ADMIN,
  ROLE_GUEST,
  ROLE_UNKNOWN,
  ROLE_USER,
  STATUS_ACTIVE,
  STATUS_BANNED,
  STATUS_DISABLED,
  roleLabel,
  roleOptions,
  statusLabel,
  statusOptions,
} from '~/utils/roles'

/**
 * 这些用例钉的是**与后端常量的一致性**。
 *
 * 起因：管理端曾把角色数值写反（1 当"普通用户"、2 当"管理员"），而后端是
 * `RoleAdmin = 1 / RoleUser = 2`。于是编辑弹窗里选「普通用户」提交 `role: 1`，
 * 后端据此把该用户**设为管理员** —— 一条提权方向的错误。
 * 当时列表里也看不到角色（后端没返回 role），界面上完全察觉不到。
 *
 * 所以这里断言的是具体数字，不是"能返回一个字符串"。
 * 如果后端改了枚举，这组用例会先红，逼着人来对齐而不是静默错位。
 *
 * 另一条来自 ADR-0017 决策 3：访客（3）与库里未入枚举的 role=0 在鉴权层视同未登录，
 * 所以它们的标签必须读出"受限"语义，**不能再显示成「注册用户」**——
 * 否则管理员会把一个写不了任何东西的账号当成正常账号。
 */
describe('角色常量与后端一致', () => {
  it('1 = 管理员', () => {
    expect(ROLE_ADMIN).toBe(1)
    expect(roleLabel(ROLE_ADMIN)).toBe('管理员')
  })

  it('2 = 注册用户', () => {
    expect(ROLE_USER).toBe(2)
    expect(roleLabel(ROLE_USER)).toBe('注册用户')
  })

  it('3 = 受限访客（无法写入）', () => {
    expect(ROLE_GUEST).toBe(3)
    expect(roleLabel(ROLE_GUEST)).toBe('受限访客（无法写入）')
  })

  it('0 = 未知角色（按受限访客处理）', () => {
    // role=0 是库里的列默认值，不在后端枚举里；鉴权层视同未登录。
    expect(ROLE_UNKNOWN).toBe(0)
    expect(roleLabel(ROLE_UNKNOWN)).toBe('未知角色（按受限访客处理）')
  })

  it('缺省值不能回落成注册用户', () => {
    // 后端没返回 role 时按最小权限处理：既不能默认成 1（管理员），
    // 也不能装作是 2（注册用户）——那会让人以为该账号能写。
    expect(roleLabel(undefined)).toBe('未知角色（按受限访客处理）')
    expect(roleLabel(0)).toBe('未知角色（按受限访客处理）')
  })

  it('枚举外的数值也按未知角色处理', () => {
    // 后端将来加了新角色而这里没跟上时，宁可显示"未知 + 受限"，
    // 也不要凭空当成注册用户或管理员。
    expect(roleLabel(99)).toBe('未知角色（按受限访客处理）')
  })

  it('下拉选项的 value 与常量一致', () => {
    expect(roleOptions.map((o) => o.value)).toEqual([ROLE_ADMIN, ROLE_USER, ROLE_GUEST])
  })

  it('下拉里保留访客，但标注受限', () => {
    const guest = roleOptions.find((o) => o.value === ROLE_GUEST)
    expect(guest?.label).toBe('受限访客（无法写入）')
  })
})

describe('状态常量与后端一致', () => {
  it('1/2/3 分别对应 正常/禁用/封禁', () => {
    expect(STATUS_ACTIVE).toBe(1)
    expect(STATUS_DISABLED).toBe(2)
    expect(STATUS_BANNED).toBe(3)
    expect(statusLabel(STATUS_ACTIVE)).toBe('正常')
    expect(statusLabel(STATUS_DISABLED)).toBe('禁用')
    expect(statusLabel(STATUS_BANNED)).toBe('封禁')
  })

  it('未知状态按正常展示', () => {
    expect(statusLabel(undefined)).toBe('正常')
    expect(statusLabel(99)).toBe('正常')
  })

  it('下拉选项的 value 与常量一致', () => {
    expect(statusOptions.map((o) => o.value)).toEqual([STATUS_ACTIVE, STATUS_DISABLED, STATUS_BANNED])
  })
})
