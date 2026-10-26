import { describe, expect, it } from 'vitest'

import { formatDateTime, formatNumber, shortId } from '~/utils/format'

describe('formatDateTime', () => {
  it('空值显示占位符', () => {
    expect(formatDateTime(undefined)).toBe('-')
    expect(formatDateTime(null)).toBe('-')
    expect(formatDateTime('')).toBe('-')
  })

  it('非法时间原样返回（不吞掉后端给的原值）', () => {
    expect(formatDateTime('not-a-date')).toBe('not-a-date')
  })

  it('合法时间按 zh-CN + 24 小时制格式化', () => {
    // 用带时区偏移的固定时间，避免受运行环境时区影响。
    const out = formatDateTime('2026-01-02T03:04:05+08:00')
    expect(out).toContain('2026')
    expect(out).toMatch(/0?3:04/)
  })
})

describe('formatNumber', () => {
  it('空值按 0 处理', () => {
    expect(formatNumber(undefined)).toBe('0')
    expect(formatNumber(null)).toBe('0')
  })

  it('千分位分隔', () => {
    expect(formatNumber(1234567)).toBe('1,234,567')
  })

  it('0 就是 0（不能因为 falsy 被当成空值）', () => {
    expect(formatNumber(0)).toBe('0')
  })
})

describe('shortId', () => {
  it('空值显示占位符', () => {
    expect(shortId(undefined)).toBe('-')
    expect(shortId(null)).toBe('-')
    expect(shortId('')).toBe('-')
  })

  it('短 ID 原样返回', () => {
    expect(shortId('123456789012')).toBe('123456789012')
  })

  it('长雪花 ID 截成 前6...后4', () => {
    expect(shortId('1234567890123456789')).toBe('123456...6789')
  })
})
