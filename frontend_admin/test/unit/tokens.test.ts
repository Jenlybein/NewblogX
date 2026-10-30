import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import tailwindConfig from '../../tailwind.config'
import { compileTailwindProbe } from '../../../scripts/tailwind-probe.mjs'

/**
 * 后台 tokens.css 的取值约定（#22 翻转后）。
 *
 * 前提变化：后台不再有自己的色相集合。它与读者站**逐字节同一份** tokens.css
 * （同步门禁见 scripts/check-token-parity.mjs），所以这里的断言是"取值口径"，
 * 而"两端一致"由 parity 门禁与 admin-flip 用例各守一遍。
 */
// vitest 以 project root（frontend_admin/）为 cwd。
const projectDir = process.cwd()
const tokensCss = readFileSync(join(projectDir, 'app/assets/css/tokens.css'), 'utf8')
const readerTokensCss = readFileSync(join(projectDir, '../frontend/app/assets/css/tokens.css'), 'utf8')
const extend = tailwindConfig.theme?.extend as Record<string, Record<string, unknown>>

function declaredNames(css: string, selectorPattern: RegExp): string[] {
  const block = selectorPattern.exec(css)
  if (!block) throw new Error(`tokens.css 里找不到 ${selectorPattern}`)
  return [...block[1].matchAll(/(--[a-z0-9-]+)\s*:/giu)].map((match) => match[1])
}

describe('原始层：与读者站同源的五色（克莱因蓝退役）', () => {
  it('五个锁定色相取到规格给定的值', () => {
    expect(tokensCss).toContain('--jelly-cream: #fdfbf6')
    expect(tokensCss).toContain('--jelly-warm: #f2eee6')
    expect(tokensCss).toContain('--jelly-yellow: #f5c518')
    expect(tokensCss).toContain('--jelly-coral: #ff6b4a')
    expect(tokensCss).toContain('--jelly-interactive: #0066cc')
  })

  it('克莱因蓝整个退役：后台旧蓝值随读者站统一到中性石墨阶', () => {
    expect(tokensCss).not.toMatch(/--jelly-blue/iu)
    expect(tokensCss).not.toMatch(/#002fa7/iu)
    expect(tokensCss).not.toMatch(/rgb\(0\s*47\s*167/iu)
    // 旧后台的蓝档（900/800/600/400 …）也随之消失。
    for (const legacy of ['#001b63', '#001f70', '#00227a', '#3a55a8', '#6e86cf', '#00113d']) {
      expect(tokensCss, legacy).not.toContain(legacy)
    }
  })

  it('不新增第六个色相（三元组基底同属已有色相）', () => {
    const hues = new Set(
      [...tokensCss.matchAll(/--jelly-([a-z0-9-]+)\s*:/giu)]
        .map((match) => match[1]!.toLowerCase())
        .filter((name) => !name.endsWith('-rgb'))
        .map((name) => name.split('-')[0]!),
    )

    expect([...hues].sort()).toEqual(['coral', 'cream', 'interactive', 'warm', 'yellow'])
  })
})

describe('语义层：明暗两份映射 + 后台特有变体已合并进主表', () => {
  it('暗色映射是同一组语义 token 的第二份取值（后台 #22 起正式启用）', () => {
    const light = declaredNames(tokensCss, /:root\s*\{([\s\S]*?)\n\s*\}/u).filter((name) =>
      name.startsWith('--color-'),
    )
    const dark = declaredNames(tokensCss, /(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\s*\}/u).filter((name) =>
      name.startsWith('--color-'),
    )

    expect(light.length).toBeGreaterThanOrEqual(20)
    expect([...dark].sort()).toEqual([...light].sort())
  })

  it('语义层包含结构线三档、高亮底、危险与危险文字、焦点环、阴影、反白层', () => {
    for (const name of [
      '--color-bg-page',
      '--color-surface',
      '--color-surface-raised',
      '--color-ink',
      '--color-ink-muted',
      '--color-ink-faint',
      '--color-line',
      '--color-line-strong',
      '--color-line-faint',
      '--color-highlight',
      '--color-danger',
      '--color-danger-ink',
      '--color-danger-soft',
      '--color-danger-line',
      '--color-focus',
      '--color-shadow',
      '--color-on-inverse',
      '--color-on-inverse-muted',
      '--color-surface-inverse',
    ]) {
      expect(tokensCss).toContain(`${name}:`)
    }
  })

  it('后台原有的特有变体也在同一张表里（不再各留一份）', () => {
    for (const name of [
      '--color-surface-glass',
      '--color-on-strong-line',
      '--color-on-strong-soft',
      '--font-mono',
    ]) {
      expect(tokensCss).toContain(`${name}:`)
    }
  })

  it('后台这份就是读者站那份（逐字节相同）', () => {
    expect(tokensCss).toBe(readerTokensCss)
  })

  it('排版层同样携带中文尺度（68ch / 1.75 / 700 / -0.03em 大标题字距）', () => {
    expect(tokensCss).toContain('--type-measure: 68ch')
    expect(tokensCss).toContain('--type-leading-prose: 1.75')
    expect(tokensCss).toContain('--type-weight-title: 700')
    expect(tokensCss).toMatch(/--type-tracking-title:\s*-0\.03em/u)
    expect(tokensCss).toContain('--font-latin: "Inter", "Inter Fallback"')
  })
})

describe('动效层：与读者站同一节奏（不接动效库）', () => {
  const adminCss = readFileSync(join(projectDir, 'app/assets/css/admin.css'), 'utf8')

  function motionDeclarations(css: string): Record<string, string> {
    const rootBlock = /:root\s*\{([\s\S]*?)\n\s*\}/u.exec(css)?.[1] ?? ''
    const declarations: Record<string, string> = {}
    for (const match of rootBlock.matchAll(/--motion-([a-z]+)\s*:\s*([^;]+);/gu)) {
      declarations[match[1]] = match[2].trim()
    }
    return declarations
  }

  it('后台补齐时长 + 单一缓动，取值与读者站一致', () => {
    const admin = motionDeclarations(tokensCss)
    const reader = motionDeclarations(readerTokensCss)

    // --motion-slow / --motion-stagger 已因零 var() 引用从两份 tokens.css 删除，
    // 现存的只有 fast / base / ease（stagger 的 delay 由 motion.ts 直接算毫秒）。
    expect(admin).toMatchObject({
      fast: '120ms',
      base: '200ms',
      ease: 'cubic-bezier(0.2, 0.8, 0.2, 1)',
    })
    // 节奏必须两端同拍，否则同一套交互在两个应用里手感不同。
    for (const name of Object.keys(admin)) {
      expect(admin[name]).toBe(reader[name])
    }
  })

  it('「减少动态」把动效 token 归零（后台也不硬动着）', () => {
    const reduceBlock = /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{([\s\S]*)$/u.exec(tokensCss)

    expect(reduceBlock).not.toBeNull()
    // --motion-slow / --motion-stagger 已因零 var() 引用从两份 tokens.css 删除。
    expect(reduceBlock?.[1]).toMatch(/--motion-fast:\s*0ms/u)
    expect(reduceBlock?.[1]).toMatch(/--motion-base:\s*0ms/u)
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
  it('暗色策略与读者站一致（class；后台现在有开关了）', () => {
    expect(tailwindConfig.darkMode).toBe('class')
  })

  it('语义色类名映射到 CSS 变量，配置里没有颜色字面量', () => {
    expect(extend.colors.ink).toEqual({
      DEFAULT: 'var(--color-ink)',
      muted: 'var(--color-ink-muted)',
      faint: 'var(--color-ink-faint)',
    })
    expect(extend.colors.surface).toBe('var(--color-surface)')
    expect(extend.colors.line).toEqual({
      DEFAULT: 'var(--color-line)',
      strong: 'var(--color-line-strong)',
      faint: 'var(--color-line-faint)',
    })
    // 原始层色相不再映射成类名（后台没有「直接用色相」的组件，克莱因蓝已退役）。
    expect(extend.colors.jelly).toBeUndefined()
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

describe('地基种子接线（圆角 / 玻璃 / 字体分工键）', () => {
  it('圆角五档、玻璃参数与字体键都从 token 取值', () => {
    expect(extend.borderRadius).toEqual({
      control: 'var(--radius-sm)',
      field: 'var(--radius-md)',
      card: 'var(--radius-lg)',
      overlay: 'var(--radius-xl)',
      pill: 'var(--radius-pill)',
    })
    expect(extend.colors.glass).toEqual({
      thin: 'var(--glass-thin-fill)',
      thick: 'var(--glass-thick-fill)',
      rim: 'var(--glass-rim)',
      'rim-warm': 'var(--glass-rim-warm)',
    })
    expect(extend.fontFamily.mono).toBe('var(--font-mono)')
    expect(extend.fontFamily.title).toBe('var(--font-title)')
  })
})
