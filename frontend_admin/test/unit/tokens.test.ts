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

describe('动效层：与读者站同一节奏（不接动效库）', () => {
  const adminCss = readFileSync(join(projectDir, 'app/assets/css/admin.css'), 'utf8')
  const readerTokensCss = readFileSync(join(projectDir, '../frontend/app/assets/css/tokens.css'), 'utf8')

  function motionDeclarations(css: string): Record<string, string> {
    const rootBlock = /:root\s*\{([\s\S]*?)\n\s*\}/u.exec(css)?.[1] ?? ''
    const declarations: Record<string, string> = {}
    for (const match of rootBlock.matchAll(/--motion-([a-z]+)\s*:\s*([^;]+);/gu)) {
      declarations[match[1]] = match[2].trim()
    }
    return declarations
  }

  it('后台补齐三档时长 + 单一缓动，取值与读者站一致', () => {
    const admin = motionDeclarations(tokensCss)
    const reader = motionDeclarations(readerTokensCss)

    expect(admin).toMatchObject({
      fast: '120ms',
      base: '200ms',
      slow: '320ms',
      ease: 'cubic-bezier(0.2, 0.8, 0.2, 1)',
    })
    // 节奏必须两端同拍，否则同一套交互在两个应用里手感不同。
    for (const name of ['fast', 'base', 'slow', 'ease']) {
      expect(admin[name]).toBe(reader[name])
    }
  })

  it('「减少动态」把动效 token 归零（后台也不硬动着）', () => {
    const reduceBlock = /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{([\s\S]*)$/u.exec(tokensCss)

    expect(reduceBlock).not.toBeNull()
    expect(reduceBlock?.[1]).toMatch(/--motion-fast:\s*0ms/u)
    expect(reduceBlock?.[1]).toMatch(/--motion-base:\s*0ms/u)
    expect(reduceBlock?.[1]).toMatch(/--motion-slow:\s*0ms/u)
  })

  it('admin.css 的过渡只消费 --motion-* 变量，没有滚动编排/关键帧', () => {
    const transitions = [...adminCss.matchAll(/transition\s*:\s*([^;]+);/gu)].map((match) =>
      match[1].replace(/\s+/gu, ' ').trim(),
    )

    expect(transitions.length).toBeGreaterThan(0)
    for (const value of transitions) {
      // 裸时长（如 `transition: all .2s`）会让三档预算在后台失守。
      expect(value).toMatch(/var\(--motion-(fast|base|slow)\)/u)
      expect(value).not.toMatch(/\b\d+(?:\.\d+)?m?s\b/u)
    }

    expect(adminCss).not.toMatch(/@keyframes|animation-timeline|scroll-timeline|scroll-behavior/u)
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
