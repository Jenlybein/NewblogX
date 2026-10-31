import { describe, expect, it } from 'vitest'

import { framingStyle, type Framing } from '~/utils/framing'

/**
 * 取景 → CSS 的**契约表**（#45 / #46 / US9）。
 *
 * 这张表与阅读站那份**逐字相同**（frontend/test/utils/bannerFramingContract.test.ts）。
 * 两个前端没有共享代码包（AGENTS.md），所以等价性不靠 import 对方，而靠两端都被
 * 钉在同一组字面量上：谁改了取整或格式化，谁自己的测试就红。
 */
const CASES: { name: string; framing: Framing }[] = [
  { name: '缺省（历史图）', framing: { focus_x: 0.5, focus_y: 0.5, zoom: 1 } },
  { name: '居中', framing: { focus_x: 0.5, focus_y: 0.5, zoom: 1 } },
  { name: '偏左上', framing: { focus_x: 0.2, focus_y: 0.15, zoom: 1.5 } },
  { name: '右上角极限', framing: { focus_x: 1, focus_y: 0, zoom: 2 } },
  { name: '非步进值', framing: { focus_x: 0.123456, focus_y: 0.987654, zoom: 1.005 } },
  { name: '非步进缩放', framing: { focus_x: 0.5, focus_y: 0.5, zoom: 1.37 } },
]

const EXPECTED: Record<string, string> = {
  '缺省（历史图）': '50% 50%',
  居中: '50% 50%',
  偏左上: '20% 15%',
  右上角极限: '100% 0%',
  非步进值: '12.35% 98.77%',
  非步进缩放: '50% 50%',
}

const EXPECTED_SCALE: Record<string, string> = {
  '缺省（历史图）': 'scale(1)',
  居中: 'scale(1)',
  偏左上: 'scale(1.5)',
  右上角极限: 'scale(2)',
  // 1.005 两位取整回落成 1：这是取整口径，两端一致即可。
  非步进值: 'scale(1)',
  非步进缩放: 'scale(1.37)',
}

describe('后台取景样式＝跨端契约表', () => {
  it.each(CASES)('$name：object/transform-origin 取同一焦点、scale 取缩放', ({ name, framing }) => {
    const style = framingStyle(framing)

    expect(style.objectFit).toBe('cover')
    expect(style.objectPosition).toBe(EXPECTED[name])
    expect(style.transformOrigin).toBe(EXPECTED[name])
    expect(style.transform).toBe(EXPECTED_SCALE[name])
  })
})
