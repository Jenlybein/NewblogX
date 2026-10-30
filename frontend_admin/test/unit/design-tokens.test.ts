import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { afterAll, describe, expect, it } from 'vitest'

import {
  checkProject,
  evaluateDrift,
  findColorLiterals,
  isExemptPath,
  loadBaseline,
} from '../../../scripts/check-design-tokens.mjs'

// vitest 以 project root（frontend_admin/）为 cwd。
const projectDir = process.cwd()
const canaryDirs: string[] = []

function makeCanaryProject(files: Record<string, string>): string {
  const dir = mkdtempSync(join(tmpdir(), 'jelly-token-canary-admin-'))
  canaryDirs.push(dir)
  for (const [relPath, content] of Object.entries(files)) {
    const full = join(dir, relPath)
    mkdirSync(join(full, '..'), { recursive: true })
    writeFileSync(full, content, 'utf8')
  }
  return dir
}

afterAll(() => {
  for (const dir of canaryDirs) rmSync(dir, { recursive: true, force: true })
})

describe('设计 token 漂移门禁（后台）', () => {
  it('抓裸十六进制、rgba() 与 Tailwind 任意值里的颜色', () => {
    const source = [
      '<template>',
      '  <div class="shadow-[inset_0_1px_0_rgba(255,255,255,0.65)]" />',
      '</template>',
      '<style>',
      '  .x { color: #0f766e; }',
      '</style>',
    ].join('\n')

    expect(findColorLiterals(source)).toEqual([
      { line: 2, match: 'rgba(255,255,255,0.65)' },
      { line: 5, match: '#0f766e' },
    ])
  })

  it('只消费 token 变量的样式算合规', () => {
    expect(findColorLiterals('.x { color: var(--color-ink); background: var(--color-surface); }')).toEqual([])
  })

  it('token 文件与品牌资产目录豁免', () => {
    expect(isExemptPath('app/assets/css/tokens.css')).toBe(true)
    expect(isExemptPath('app/assets/brand/JellyMark.vue')).toBe(true)
    expect(isExemptPath('app/components/admin/AdminShell.vue')).toBe(false)
  })

  it('人造违规文件会让门禁失败（canary：后台这份门禁也会红）', () => {
    const result = evaluateDrift({
      violations: [{ file: 'app/components/__canary.vue', line: 4, match: '#ff0000' }],
      baselineFiles: [],
      scannedFiles: ['app/components/__canary.vue'],
    })

    expect(result.ok).toBe(false)
    expect(result.failures.map((failure) => failure.kind)).toEqual(['new-color-literal'])
  })

  it('baseline 里已清干净的文件必须删掉条目（只许收缩）', () => {
    const result = evaluateDrift({
      violations: [],
      baselineFiles: ['app/components/admin/OldDirty.vue'],
      scannedFiles: ['app/components/admin/OldDirty.vue'],
    })

    expect(result.ok).toBe(false)
    expect(result.failures).toEqual([{ kind: 'baseline-should-shrink', file: 'app/components/admin/OldDirty.vue' }])
  })

  it('真实后台当前处于 baseline 允许范围内（门禁为绿）', () => {
    const result = checkProject({ projectDir, baselineFiles: loadBaseline(projectDir) })

    // 后台现在只剩 2 个存量文件；断言 >0 是为了确保这条用例真的在扫东西，
    // 否则"扫描器返回空 → 门禁绿"这种失效模式会被漏掉。
    expect(result.violations.length).toBeGreaterThan(0)
    expect(result.ok).toBe(true)
  })

  it('临时目录里放真实违规文件时会被报出（不是永远为绿）', () => {
    const canaryProject = makeCanaryProject({
      'app/components/admin/Canary.vue': '<style>.canary { border-color: #ff0000; }</style>\n',
    })

    const result = checkProject({ projectDir: canaryProject, baselineFiles: [] })

    expect(result.violations).toEqual([
      { file: 'app/components/admin/Canary.vue', line: 1, match: '#ff0000' },
    ])
    expect(result.ok).toBe(false)
  })
})
