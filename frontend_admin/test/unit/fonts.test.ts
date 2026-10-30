import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

// vitest 以 project root（frontend_admin/）为 cwd。
const projectDir = process.cwd()
const fontsCss = readFileSync(join(projectDir, 'app/assets/css/fonts.css'), 'utf8')
const adminCss = readFileSync(join(projectDir, 'app/assets/css/admin.css'), 'utf8')
const appVue = readFileSync(join(projectDir, 'app/app.vue'), 'utf8')

const faces = [...fontsCss.matchAll(/@font-face\s*\{([^}]*)\}/gu)].map((match) => match[1].trim())

function facesOfFamily(family: string): string[] {
  const pattern = new RegExp(`font-family:\\s*["']${family}["']`, 'u')
  return faces.filter((face) => pattern.test(face))
}

describe('后台中文自托管字体', () => {
  it('Noto Sans SC 的 400 / 700 两档都有 @font-face', () => {
    const weights = facesOfFamily('Noto Sans SC').map(
      (face) => /\bfont-weight:\s*(\d+)/u.exec(face)?.[1] ?? '?',
    )

    expect(weights.sort()).toEqual(['400', '700'])
  })

  it('src 指向的文件真实存在（拼错路径 = 字体静默不加载）', () => {
    for (const face of facesOfFamily('Noto Sans SC')) {
      const urls = [...face.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/gu)].map((match) => match[1])

      for (const url of urls) {
        expect(url).toMatch(/^@fontsource\/noto-sans-sc\/files\//u)
        expect(existsSync(join(projectDir, 'node_modules', url))).toBe(true)
      }
      expect(face).toMatch(/font-display:\s*swap/u)
    }
  })

  it('配有度量匹配 fallback（换字不跳版）', () => {
    const fallback = facesOfFamily('Noto Sans SC Fallback')[0]

    expect(fallback).toBeDefined()
    expect(fallback).toMatch(/src:\s*local\(/u)
    expect(fallback).toMatch(/ascent-override:\s*[\d.]+%/u)
    expect(fallback).toMatch(/descent-override:\s*[\d.]+%/u)
    expect(fallback).toMatch(/line-gap-override:\s*0%/u)
    expect(fallback).toMatch(/size-adjust:\s*100%/u)
  })

  it('body 与 Naive themeOverrides 都消费中文组', () => {
    expect(adminCss).toMatch(/font-family:\s*var\(--font-sans\)/u)

    const stack = appVue.slice(appVue.indexOf('fontFamily:'), appVue.indexOf('fontFamily:') + 320)
    expect(stack).toContain('"Noto Sans SC"')
    expect(stack.indexOf('"Noto Sans SC"')).toBeLessThan(stack.indexOf('"PingFang SC"'))
  })
})
