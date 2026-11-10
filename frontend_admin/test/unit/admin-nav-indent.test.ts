import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

import { ADMIN_NAV_TREE } from '../../app/utils/menu'

/**
 * 侧栏缩进的层级契约（ADR-0039）。
 *
 * 起因是线上的真实截图：「站点配置（一级）」与「站点（二级）」缩进一模一样，
 * 而「站点」的子项却缩进得很深——层级读起来像"子项挂在站点配置下面"。
 *
 * 根因是分组行与链接行用的是两套元素：
 *   链接 → `.admin-nav__link`（px-3，depth 缩进写在它身上）
 *   分组 → `.admin-nav__group`（px-2）+ `.admin-nav__group-title`（px-1）
 * 只给链接写 depth 缩进时，二级分组完全不受影响。
 *
 * CSS 不进 happy-dom 的计算样式，所以这里钉的是**规则的存在与递增关系**，
 * 常量值与实际像素的对应关系写在 admin.css 的注释里。
 */

const css = readFileSync(join(process.cwd(), 'app', 'assets', 'css', 'admin.css'), 'utf8')

/** 取出某条选择器规则里的 padding-left 值（px → rem 数值）。 */
function paddingLeftOf(selector: string): number | null {
  const start = css.indexOf(selector)
  if (start === -1) return null
  const body = css.slice(start, css.indexOf('}', start))
  const match = body.match(/padding-left:\s*([0-9.]+)rem/u)
  return match ? Number(match[1]) : null
}

describe('侧栏缩进：每个层级都要同时缩进「链接行」与「分组行」', () => {
  it('一级 / 二级的链接与分组都有 depth 规则（分组行不能漏）', () => {
    for (const depth of ['1', '2']) {
      for (const el of ['admin-nav__link', 'admin-nav__group']) {
        const selector = `.admin-nav__item[data-depth='${depth}'] > .${el} {`
        expect(paddingLeftOf(selector), `${selector} 缺少缩进规则`).not.toBeNull()
      }
    }
  })

  it('缩进随层级严格递增（同一层级的链接与分组对齐同一条竖线）', () => {
    const link1 = paddingLeftOf(".admin-nav__item[data-depth='1'] > .admin-nav__link {")
    const link2 = paddingLeftOf(".admin-nav__item[data-depth='2'] > .admin-nav__link {")
    const group1 = paddingLeftOf(".admin-nav__item[data-depth='1'] > .admin-nav__group {")
    const group2 = paddingLeftOf(".admin-nav__item[data-depth='2'] > .admin-nav__group {")

    expect(link1).not.toBeNull()
    expect(link2).not.toBeNull()
    expect(group1).not.toBeNull()
    expect(group2).not.toBeNull()

    // 二级必须比一级更深，否则层级看不出来（这次 bug 的判据）。
    expect(group2!).toBeGreaterThan(group1!)
    expect(link2!).toBeGreaterThan(link1!)

    // 分组行比同层链接少 0.25rem，正好吃掉 group-title 的 px-1，
    // 于是两者的图标落在同一条竖线上（差 0.5rem 就会明显错位）。
    expect(link1! - group1!).toBe(0.25)
    expect(link2! - group2!).toBe(0.25)
  })

  it('真实菜单里存在二级分组，这条规则不是纸上需求', () => {
    const nested: { label: string; children?: unknown[] }[] = []
    const walk = (nodes: typeof ADMIN_NAV_TREE, depth: number) => {
      for (const node of nodes) {
        if (depth >= 1 && (node.children?.length ?? 0) > 0) nested.push(node)
        if (node.children?.length) walk(node.children as typeof ADMIN_NAV_TREE, depth + 1)
      }
    }
    walk(ADMIN_NAV_TREE, 0)

    expect(nested.map((node) => node.label)).toContain('站点')
  })
})
