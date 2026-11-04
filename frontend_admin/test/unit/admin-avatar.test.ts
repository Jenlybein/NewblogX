import { describe, expect, it } from 'vitest'

import {
  isEmailLikeName,
  resolveAdminAvatarInitial,
  resolveAdminAvatarUrl,
} from '../../app/utils/adminAvatar'

/**
 * 后台头像兜底（决定清单 §6 第 28 条）。
 *
 * 三件事必须钉死，否则列表里会出现「一片空白」或「一个 1」这种假头像：
 *   1. 邮箱形式的用户名不取首字符（后台一半账号的用户名是邮箱）；
 *   2. 地址补全只补一次（"/uploads/a.png" 不能变成 "/uploads/uploads/a.png"）；
 *   3. 绝对地址 / data: / blob: 原样放过（已经可直接用）。
 */

describe('isEmailLikeName', () => {
  const cases: Array<[unknown, boolean]> = [
    ['1813288952@qq.com', true],
    ['  Jen@Example.COM  ', true],
    ['jen', false],
    ['@jen', false],
    ['jen@', false],
    ['两个 空格 @x.com', false],
    ['', false],
    [null, false],
    [undefined, false],
    [42, false],
  ]

  for (const [input, expected] of cases) {
    it(`${JSON.stringify(input)} → ${expected}`, () => {
      expect(isEmailLikeName(input)).toBe(expected)
    })
  }
})

describe('resolveAdminAvatarUrl', () => {
  const cases: Array<[unknown, string]> = [
    // 空值一律空串：调用方据此回退到首字母 / 图标，而不是发一个空 src。
    ['', ''],
    ['   ', ''],
    [null, ''],
    [undefined, ''],
    // 已经带基址的不重复拼。
    ['/uploads/a.png', '/uploads/a.png'],
    // 站内相对路径补基址（带不带前导斜杠都一样）。
    ['/a.png', '/uploads/a.png'],
    ['a.png', '/uploads/a.png'],
    // 绝对地址 / 内联数据原样放过。
    ['https://cdn.test/a.png', 'https://cdn.test/a.png'],
    ['//cdn.test/a.png', '//cdn.test/a.png'],
    ['data:image/png;base64,AAA', 'data:image/png;base64,AAA'],
    ['blob:http://localhost/abc', 'blob:http://localhost/abc'],
  ]

  for (const [input, expected] of cases) {
    it(`${JSON.stringify(input)} → ${JSON.stringify(expected)}`, () => {
      expect(resolveAdminAvatarUrl(input)).toBe(expected)
    })
  }
})

describe('resolveAdminAvatarInitial', () => {
  it('中文名取首字、英文名首字母大写', () => {
    expect(resolveAdminAvatarInitial('张三')).toBe('张')
    expect(resolveAdminAvatarInitial('jen')).toBe('J')
    expect(resolveAdminAvatarInitial('  JEN  ')).toBe('J')
  })

  it('邮箱形式一律不取首字符（否则会画出一个 "1"）', () => {
    expect(resolveAdminAvatarInitial('1813288952@qq.com')).toBe('')
    expect(resolveAdminAvatarInitial('1813288952@qq.com', 'A')).toBe('A')
  })

  it('空名 / 非字符串回退到 fallback', () => {
    expect(resolveAdminAvatarInitial('')).toBe('')
    expect(resolveAdminAvatarInitial('   ')).toBe('')
    expect(resolveAdminAvatarInitial(null)).toBe('')
    expect(resolveAdminAvatarInitial(undefined, '?')).toBe('?')
  })
})
