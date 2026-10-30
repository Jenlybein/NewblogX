import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import tailwindConfig from '../../tailwind.config'
import { compileTailwindProbe } from '../../../scripts/tailwind-probe.mjs'

// vitest 以 project root（frontend_admin/）为 cwd。
const projectDir = process.cwd()
const tokensCss = readFileSync(join(projectDir, 'app/assets/css/tokens.css'), 'utf8')
const extend = tailwindConfig.theme?.extend as Record<string, Record<string, unknown>>

function declaredNames(css: string, selectorPattern: RegExp): string[] {
  const block = selectorPattern.exec(css)
  if (!block) throw new Error(`tokens.css 里找不到 ${selectorPattern}`)
  return [...block[1].matchAll(/(--[a-z0-9-]+)\s*:/giu)].map((match) => match[1])
}

describe('原始层：与读者站同源的五色锁死', () => {
  it('五个锁定色相取到 spec 给定的值', () => {
    expect(tokensCss).toContain('--jelly-blue: #002fa7')
    expect(tokensCss).toContain('--jelly-cream: #fdfbf6')
    expect(tokensCss).toContain('--jelly-yellow: #f5c518')
    expect(tokensCss).toContain('--jelly-coral: #ff6b4a')
    expect(tokensCss).toContain('--jelly-warm: #f2eee6')
  })

  it('不新增第六个色相', () => {
    const hues = new Set(
      [...tokensCss.matchAll(/--jelly-([a-z]+)[a-z0-9-]*\s*:/giu)].map((match) => match[1].toLowerCase()),
    )

    expect([...hues].sort()).toEqual(['blue', 'coral', 'cream', 'warm', 'yellow'])
  })
})

describe('语义层：明暗两份映射 + 排版尺度', () => {
  it('暗色映射是同一组语义 token 的第二份取值（后台暂不切暗色，但取值先备好）', () => {
    const light = declaredNames(tokensCss, /:root\s*\{([\s\S]*?)\n\s*\}/u).filter((name) =>
      name.startsWith('--color-'),
    )
    const dark = declaredNames(tokensCss, /(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\s*\}/u).filter((name) =>
      name.startsWith('--color-'),
    )

    expect(light.length).toBeGreaterThanOrEqual(13)
    expect([...dark].sort()).toEqual([...light].sort())
  })

  it('语义层包含结构线三档、高亮底、危险与危险文字、焦点环、阴影', () => {
    for (const name of [
      '--color-bg-page',
      '--color-surface',
      '--color-ink',
      '--color-ink-muted',
      '--color-line',
      '--color-line-strong',
      '--color-line-faint',
      '--color-highlight',
      '--color-danger',
      '--color-danger-ink',
      '--color-focus',
      '--color-shadow',
    ]) {
      expect(tokensCss).toContain(`${name}:`)
    }
  })

  it('排版层同样携带中文尺度（68ch / 1.75 / 700 / 负字距）', () => {
    expect(tokensCss).toContain('--type-measure: 68ch')
    expect(tokensCss).toContain('--type-leading-prose: 1.75')
    expect(tokensCss).toContain('--type-weight-title: 700')
    expect(tokensCss).toMatch(/--type-tracking-title:\s*-0\.0[0-9]em/u)
  })
})

describe('Tailwind 配置：色板与暗色策略', () => {
  it('暗色策略与读者站一致（class；后台目前没有开关，但不能是 media）', () => {
    expect(tailwindConfig.darkMode).toBe('class')
  })

  it('语义色类名映射到 CSS 变量，配置里没有颜色字面量', () => {
    expect(extend.colors.ink).toEqual({ DEFAULT: 'var(--color-ink)', muted: 'var(--color-ink-muted)' })
    expect(extend.colors.surface).toBe('var(--color-surface)')
    expect(extend.colors.line).toEqual({
      DEFAULT: 'var(--color-line)',
      strong: 'var(--color-line-strong)',
      faint: 'var(--color-line-faint)',
    })
    expect(JSON.stringify(tailwindConfig)).not.toMatch(/#[0-9a-f]{3,8}/iu)
  })

  it('可编译：语义色与 dark: 工具类都真的生成 CSS', async () => {
    const css = await compileTailwindProbe({
      projectDir,
      html: '<div class="bg-surface text-ink border-line-faint dark:bg-page"></div>',
    })

    expect(css).toContain('background-color: var(--color-surface)')
    expect(css).toContain('color: var(--color-ink)')
    expect(css).toContain('border-color: var(--color-line-faint)')
    expect(css).toMatch(/\.dark\\:bg-page:is\(\.dark \*\)/u)
    expect(css).not.toContain('prefers-color-scheme: dark')
  })
})
