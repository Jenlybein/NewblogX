import { readdirSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { ADMIN_NAV, pageFileCandidates } from '../../app/utils/menu'

/**
 * 侧边栏菜单 ↔ 页面文件对账。
 *
 * 起因：上一轮侧边栏一直挂着「日志中心 → /logs」，而 `app/pages/logs/` 根本不存在，
 * 点进去 404。没有任何检查会报错 —— 入口存在、目标不存在，只有用户点得到。
 *
 * 这条用例把它变成硬约束：菜单里的每个 path，必须在 app/pages 下有一个真实文件。
 */

// 用 cwd 而不是 import.meta.url：vitest 在 happy-dom 环境下 import.meta.url
// 不是 file:// 协议，fileURLToPath 会直接抛错。测试由包根的 pnpm test 触发，
// cwd 稳定是 frontend_admin/。
const pagesDir = join(process.cwd(), 'app', 'pages')

/** 递归收集 app/pages 下所有 .vue，返回相对路径（不含扩展名、统一用 /）。 */
function collectPageFiles(dir: string, prefix = ''): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const rel = prefix ? `${prefix}/${entry.name}` : entry.name
    if (entry.isDirectory()) {
      out.push(...collectPageFiles(join(dir, entry.name), rel))
      continue
    }
    if (entry.name.endsWith('.vue')) {
      out.push(rel.slice(0, -'.vue'.length))
    }
  }
  return out
}

describe('后台菜单', () => {
  const pageFiles = collectPageFiles(pagesDir)

  it('每个菜单 path 都有对应的页面文件', () => {
    const missing: string[] = []
    for (const item of ADMIN_NAV) {
      const candidates = pageFileCandidates(item.path)
      if (!candidates.some(candidate => pageFiles.includes(candidate))) {
        missing.push(`${item.label} ${item.path}（找过：${candidates.join(' / ')}）`)
      }
    }
    expect(missing, `菜单指向了不存在的页面：\n${missing.join('\n')}`).toEqual([])
  })

  it('pageFileCandidates 覆盖各种边界写法', () => {
    // 根路径
    expect(pageFileCandidates('/')).toEqual(['index'])
    expect(pageFileCandidates('')).toEqual(['index'])
    // 空值/纯空白：菜单里不该出现，但函数不能因此崩掉
    expect(pageFileCandidates(null as unknown as string)).toEqual(['index'])
    expect(pageFileCandidates('   ')).toEqual(['index'])
    // 只有斜杠：去掉首尾之后为空
    expect(pageFileCandidates('///')).toEqual(['index'])
    // 普通路径：两种 Nuxt 写法都作为候选
    expect(pageFileCandidates('/logs/')).toEqual(['logs', 'logs/index'])
    expect(pageFileCandidates('logs/dead-letter')).toEqual(['logs/dead-letter', 'logs/dead-letter/index'])
  })

  it('path 不重复、且都是绝对路径', () => {
    const paths = ADMIN_NAV.map(item => item.path)
    expect(new Set(paths).size).toBe(paths.length)
    for (const path of paths) {
      expect(path.startsWith('/'), `${path} 应是绝对路径`).toBe(true)
    }
  })

  it('每个页面都能从菜单到达（除登录页）', () => {
    // 反向检查：有页面但没入口 = 藏起来的死页面。登录页是例外（它不该出现在菜单里）。
    const reachable = new Set<string>()
    for (const item of ADMIN_NAV) {
      for (const candidate of pageFileCandidates(item.path)) {
        reachable.add(candidate)
      }
    }
    const unreachable = pageFiles.filter(file => file !== 'login' && !reachable.has(file))
    expect(unreachable, `这些页面没有任何菜单入口：\n${unreachable.join('\n')}`).toEqual([])
  })
})
