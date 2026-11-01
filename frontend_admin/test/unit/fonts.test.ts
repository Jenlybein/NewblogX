import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import { buildNaiveThemeOverrides, parseThemePalette, parseThemeScale, parseThemeTriplets } from '../../app/utils/theme'

// vitest 以 project root（frontend_admin/）为 cwd。
const projectDir = process.cwd()
const repoRoot = join(projectDir, '..')
const fontsCss = readFileSync(join(projectDir, 'app/assets/css/fonts.css'), 'utf8')
const adminCss = readFileSync(join(projectDir, 'app/assets/css/admin.css'), 'utf8')
const tokensCss = readFileSync(join(projectDir, 'app/assets/css/tokens.css'), 'utf8')
const appVue = readFileSync(join(projectDir, 'app/app.vue'), 'utf8')
const wordmarkVue = readFileSync(
  join(projectDir, 'app/components/admin/BrandWordmark.vue'),
  'utf8',
)
const readerTokensCss = readFileSync(
  join(repoRoot, 'frontend/app/assets/css/tokens.css'),
  'utf8',
)
const readerFontsCss = readFileSync(join(repoRoot, 'frontend/app/assets/css/fonts.css'), 'utf8')

const themeOverrides = buildNaiveThemeOverrides(
  parseThemePalette(tokensCss, 'light'),
  parseThemeTriplets(tokensCss, 'light'),
  parseThemeScale(tokensCss),
)

const faces = [...fontsCss.matchAll(/@font-face\s*\{([^}]*)\}/gu)].map((match) => match[1].trim())

function facesOfFamily(css: string, family: string): string[] {
  const pattern = new RegExp(`font-family:\\s*["']${family}["']`, 'u')
  return [...css.matchAll(/@font-face\s*\{([^}]*)\}/gu)]
    .map((match) => match[1].trim())
    .filter((face) => pattern.test(face))
}

/** 取 `:root` 块的声明值（去掉注释与多余空白）。 */
function declaration(css: string, name: string): string {
  const root = /:root\s*\{([\s\S]*?)\n\}/u.exec(css)?.[1] ?? ''
  const match = new RegExp(`${name}\\s*:\\s*([^;]+);`, 'u').exec(root)
  return (match?.[1] ?? '').replace(/\s+/gu, ' ').trim()
}

/** 取某条 CSS 规则体（只用于结构断言）。 */
function ruleBody(css: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/gu, '\\$&')
  const match = new RegExp(`${escaped}\\s*\\{([^}]*)\\}`, 'u').exec(css)
  if (!match) throw new Error(`找不到 CSS 规则 ${selector}`)
  return match[1]!
}

function stripComments(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//gu, '')
}

/** 全仓字体 token 引用（两端 app + 两端 Tailwind 配置）。 */
function fontTokenReferences(): { file: string; token: string }[] {
  const references: { file: string; token: string }[] = []
  const scan = (file: string) => {
    const text = stripComments(readFileSync(file, 'utf8'))
    for (const match of text.matchAll(/var\((--font-[a-z0-9-]+)\)/gu)) {
      references.push({ file, token: match[1]! })
    }
  }
  const walk = (dir: string) => {
    for (const name of readdirSync(dir)) {
      if (name === 'node_modules' || name === '.nuxt' || name === '.output') continue
      const full = join(dir, name)
      const info = statSync(full)
      if (info.isDirectory()) {
        walk(full)
        continue
      }
      if (/\.(vue|css|ts)$/u.test(name) && !name.endsWith('.d.ts')) scan(full)
    }
  }

  walk(join(repoRoot, 'frontend/app'))
  walk(join(projectDir, 'app'))
  scan(join(repoRoot, 'frontend/tailwind.config.ts'))
  scan(join(projectDir, 'tailwind.config.ts'))

  return references
}

/** 全部 `font-family` 声明值（含两端 fonts.css 的 @font-face）。 */
function fontFamilyDeclarations(): { file: string; family: string }[] {
  const declarations: { file: string; family: string }[] = []
  const scan = (file: string) => {
    const text = stripComments(readFileSync(file, 'utf8'))
    for (const match of text.matchAll(/font-family:\s*([^;}]+)[;}]/gu)) {
      declarations.push({ file, family: match[1]!.replace(/\s+/gu, ' ').trim() })
    }
  }
  const walk = (dir: string) => {
    for (const name of readdirSync(dir)) {
      if (name === 'node_modules' || name === '.nuxt' || name === '.output') continue
      const full = join(dir, name)
      const info = statSync(full)
      if (info.isDirectory()) {
        walk(full)
        continue
      }
      if (/\.(vue|css|ts)$/u.test(name) && !name.endsWith('.d.ts')) scan(full)
    }
  }

  scan(join(projectDir, 'app/assets/css/fonts.css'))
  scan(join(repoRoot, 'frontend/app/assets/css/fonts.css'))
  walk(join(projectDir, 'app'))
  walk(join(repoRoot, 'frontend/app'))

  return declarations
}

describe('后台中文字体：跟随读者站走系统字体栈（token 同源）', () => {
  it('fonts.css 不再有 CJK @font-face（Noto 两个整包已删除）', () => {
    // 反向断言：把 Noto 两档 @font-face 加回来就会红。
    expect(facesOfFamily(fontsCss, 'Noto Sans SC')).toHaveLength(0)
    expect(facesOfFamily(fontsCss, 'Noto Sans SC Fallback')).toHaveLength(0)
    expect(fontsCss).not.toMatch(/noto-sans-sc/iu)
  })

  it('src 指向的文件真实存在（拼错路径 = 字体静默不加载）', () => {
    for (const face of faces) {
      const urls = [...face.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/gu)].map((match) => match[1])

      if (urls.length === 0) continue
      for (const url of urls) {
        expect(url).toMatch(/^@fontsource(-variable)?\//u)
        expect(existsSync(join(projectDir, 'node_modules', url!))).toBe(true)
      }
    }
  })

  it('body 与 Naive themeOverrides 的中文都来自系统字体栈', () => {
    expect(adminCss).toMatch(/font-family:\s*var\(--font-sans\)/u)

    // 主题层由 app/utils/theme.ts 从 tokens.css 解析后组装（见 theme.test.ts）。
    const stack = String(themeOverrides.common?.fontFamily)
    expect(stack).toContain('"PingFang SC"')
    expect(stack).not.toContain('"Noto Sans SC Fallback"')
    expect(stack.indexOf('"PingFang SC"')).toBeLessThan(stack.indexOf('"Noto Sans SC"'))
  })
})

describe('后台 Inter 自托管（与读者站同一份资产）', () => {
  it('Inter 可变字体 100–900 + swap，并指向 @fontsource-variable/inter 的 woff2', () => {
    const interFaces = facesOfFamily(fontsCss, 'Inter')

    expect(interFaces).toHaveLength(1)
    expect(interFaces[0]).toMatch(/font-weight:\s*100 900/u)
    expect(interFaces[0]).toMatch(/font-display:\s*swap/u)
    expect(interFaces[0]).toContain(
      '@fontsource-variable/inter/files/inter-latin-wght-normal.woff2',
    )
  })

  it('Inter 度量匹配 fallback 与读者站逐条一致', () => {
    const adminFallback = facesOfFamily(fontsCss, 'Inter Fallback')[0]
    const readerFallback = facesOfFamily(readerFontsCss, 'Inter Fallback')[0]

    expect(adminFallback).toBeDefined()
    expect(readerFallback).toBeDefined()
    for (const descriptor of [
      'ascent-override',
      'descent-override',
      'line-gap-override',
      'size-adjust',
    ]) {
      const pattern = new RegExp(`${descriptor}:\\s*([\\d.]+)%`, 'u')
      expect(pattern.exec(adminFallback!)?.[1], descriptor).toBe(
        pattern.exec(readerFallback!)?.[1],
      )
    }
  })

  it('首屏 preload 关键可变字体文件（后台无 Sora 800 档，只 preload Inter）', () => {
    expect(appVue).toContain('inter-latin-wght-normal.woff2')
    expect(appVue).toMatch(/rel:\s*"preload"/u)
    expect(appVue).toMatch(/as:\s*"font"/u)
    expect(appVue).toContain('crossorigin: "anonymous"')
  })
})

describe('后台字体分工 token（与读者站同源）', () => {
  it('display=Sora（含中文回退）、latin=Inter、title 取 display', () => {
    const display = declaration(tokensCss, '--font-display')

    expect(display.startsWith('"Sora", "Sora Fallback"')).toBe(true)
    expect(display).toContain('var(--font-cjk)')
    expect(declaration(tokensCss, '--font-latin')).toBe('"Inter", "Inter Fallback"')
    expect(declaration(tokensCss, '--font-title')).toBe('var(--font-display)')
  })

  it('字体 token 与读者站一致（改一处必须两端同改）', () => {
    for (const token of [
      '--font-display',
      '--font-latin',
      '--font-title',
      '--font-cjk',
      '--font-sans',
      '--font-mono',
      '--type-tracking-title',
    ]) {
      expect(declaration(tokensCss, token), token).toBe(declaration(readerTokensCss, token))
    }
  })

  it('--font-sans 先 Inter 后系统中文，大标题字距 -0.03em', () => {
    const sans = declaration(tokensCss, '--font-sans')

    expect(sans).toBe('var(--font-latin), var(--font-cjk)')
    expect(declaration(tokensCss, '--type-tracking-title')).toBe('-0.03em')
  })

  it('页面标题与字标取 display 族', () => {
    expect(ruleBody(adminCss, '.admin-page-title')).toContain('font-title')
    expect(wordmarkVue).toMatch(/font-display|font-title/u)
    expect(wordmarkVue).not.toContain('font-latin font-title')
  })

  it('Naive 主题字体栈不再是 Sora（按钮/表单走 Inter + 系统中文）', () => {
    const stack = String(themeOverrides.common?.fontFamily)

    expect(stack.startsWith('"Inter"')).toBe(true)
    expect(stack).not.toContain('"Sora"')
    expect(stack.indexOf('"Inter"')).toBeLessThan(stack.indexOf('"PingFang SC"'))
  })
})

describe('后台等宽文本消费 --font-mono', () => {
  it('.admin-mono 走等宽 token（不再掉回 sans）', () => {
    const mono = declaration(tokensCss, '--font-mono')

    expect(mono).toContain('ui-monospace, monospace')
    expect(ruleBody(adminCss, '.admin-mono')).toContain('font-family: var(--font-mono)')
  })
})

describe('后台同样：无衬线、无 CDN、无未定义字体 token', () => {
  it('字体全部本地自托管（两端 fonts.css 都无远程地址）', () => {
    for (const css of [fontsCss, readerFontsCss]) {
      expect(css).not.toMatch(/https?:/u)
      for (const match of css.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/gu)) {
        expect(match[1]).toMatch(/^@fontsource(-variable)?\//u)
      }
    }
  })

  it('没有任何衬线体声明（全站无衬线）', () => {
    const declarations = fontFamilyDeclarations()

    expect(declarations.length).toBeGreaterThan(0)
    for (const { file, family } of declarations) {
      expect(family, file).not.toMatch(/(?<!sans-)serif/iu)
      expect(family, file).not.toMatch(/Georgia|Times New Roman|SimSun|Songti/iu)
    }
  })

  it('除 fonts.css 外所有 font-family 都消费 token', () => {
    for (const { file, family } of fontFamilyDeclarations()) {
      if (file.endsWith('fonts.css')) continue
      expect(family, file).toMatch(/var\(--font-/u)
    }
  })

  it('全仓不再引用未定义的字体 token', () => {
    const declared = new Set(
      [...tokensCss.matchAll(/--font-[a-z0-9-]+\s*:/gu)].map((match) =>
        match[0].replace(/\s*:/u, ''),
      ),
    )
    const references = fontTokenReferences()

    expect(references.length).toBeGreaterThan(0)
    for (const { file, token } of references) {
      expect(declared, `${file} 引用了 ${token}`).toContain(token)
    }
  })
})
