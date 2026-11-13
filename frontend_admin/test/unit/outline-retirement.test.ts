import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { ADMIN_NAV, ADMIN_NAV_TREE, FLAT_NAV } from '../../app/utils/menu'

/**
 * 后台编排入口退役门禁（#218）。
 *
 * 「旧扁平收录表不再被写入」不是靠一句文档承诺保证的：两个排列页与列表页的
 * 「收录管理」弹窗都直接调旧收录 / 排序 / 清理接口，只要页面还在、菜单还能
 * 点进去，旧表就仍会被写。这里把"退役"钉成静态规则：
 * 页面文件不存在、菜单无残留入口、后台 app 不出现旧接口与旧工具名，
 * 并且目录编排仍是唯一的编排入口。
 */

const projectDir = process.cwd()
const pagesDir = join(projectDir, 'app', 'pages')
const articlesDir = join(pagesDir, 'articles')
const adminService = withoutComments(readFileSync(join(projectDir, 'app/services/admin.ts'), 'utf8'))
const sourcesUnder = (dir: string): Array<{ file: string; text: string }> => {
  const out: Array<{ file: string; text: string }> = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) {
      out.push(...sourcesUnder(path))
      continue
    }
    if (entry.name.endsWith('.vue') || entry.name.endsWith('.ts')) {
      out.push({ file: path, text: readFileSync(path, 'utf8') })
    }
  }
  return out
}
const appSources = sourcesUnder(join(projectDir, 'app')).map(({ file, text }) => ({ file, text: withoutComments(text) }))

/** 旧扁平收录 / 排序接口名，以及旧编排纯工具名。 */
const LEGACY_SERVICE_NAMES = [
  'getAdminCollectionItems',
  'getAdminProjectItems',
  'batchIncludeCollectionArticles',
  'batchIncludeProjectArticles',
  'removeCollectionArticle',
  'removeProjectArticle',
  'cleanupCollectionStale',
  'cleanupProjectStale',
  'updateCollectionItemOrder',
  'updateProjectItemOrder',
  'updateCollectionOrder',
  'updateProjectOrder',
]
const LEGACY_UTILS = ['collectionArrange', 'projectArrange']

/** 判定"确实调用了接口"，而不是把解释性注释也算成残留。 */
function withoutComments(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//gu, '')
    .replace(/^\s*\/\/.*$/gmu, '')
    .replace(/<!--[\s\S]*?-->/gu, '')
}

describe('旧排列页与收录弹窗已退役', () => {
  it('三个旧编排页都不存在：编排收进合集 / 项目编辑弹窗', () => {
    expect(existsSync(join(articlesDir, 'collection-arrange.vue'))).toBe(false)
    expect(existsSync(join(articlesDir, 'project-arrange.vue'))).toBe(false)
    // 决定清单 §8 第 35 条：独立的「目录编排」页也退役，编排只存在于编辑弹窗的抽屉里。
    expect(existsSync(join(articlesDir, 'outline.vue'))).toBe(false)

    // 唯一的编排实现是共用组件，由编辑弹窗按 kind 复用（合集 / 项目各一次）。
    const overlay = readFileSync(join(projectDir, 'app/components/articles/CollectionEditorOverlay.vue'), 'utf8')
    expect(overlay).toContain('<OutlineTreeEditor')
    expect(overlay).toContain(':kind="kind"')
  })

  it('合集 / 项目列表页不再有「收录管理」弹窗', () => {
    for (const page of ['collections.vue', 'projects.vue']) {
      const source = withoutComments(readFileSync(join(articlesDir, page), 'utf8'))
      expect(source, page).not.toContain('收录管理')
      expect(source, page).not.toContain('NModal')
      expect(source, page).not.toContain('staleItems')
    }
  })

  it('菜单没有旧排列入口，也没有独立的编排入口', () => {
    const paths = ADMIN_NAV.map(item => item.path)
    expect(paths).not.toContain('/articles/collection-arrange')
    expect(paths).not.toContain('/articles/project-arrange')
    expect(paths).not.toContain('/articles/outline')
    expect(paths.filter(path => path.startsWith('/articles/'))).toEqual([
      '/articles/hot-top',
      '/articles/collections',
      '/articles/projects',
      // 标签管理是文章域的新维护页（#322），不是旧的排列 / 编排入口。
      '/articles/tags',
    ])
    // 编排不再是菜单项：它是合集 / 项目卡片上的「编排目录树」按钮。
    expect(FLAT_NAV.filter(item => item.label.includes('编排'))).toEqual([])
    const articleGroup = ADMIN_NAV_TREE.find(node => node.label === '文章管理')!
    expect(articleGroup.children!.filter(child => child.label.includes('编排'))).toEqual([])
  })

  it('页面数量硬断言：编排页退役后 23 个；站点拆四页与媒体拆两页后 28 个；新增短信页后 29 个（ADR-0029）', () => {
    const pages: string[] = []
    const walk = (dir: string) => {
      for (const entry of readdirSync(dir, { withFileTypes: true })) {
        const path = join(dir, entry.name)
        if (entry.isDirectory()) walk(path)
        else if (entry.name.endsWith('.vue')) pages.push(path)
      }
    }
    walk(pagesDir)
    // 23 → 28：站点拆出 login / publish / chat 三页（基础页复用 /site），
    // 媒体运营拆出 banners / images 两页（/media 保留为跳转页）。
    expect(pages.length).toBe(29)
  })

  /**
   * `utils/staleItems.ts` 只剩 `staleReasonLabel` 一个宿主（树编辑器的失效标注），
   * 失效项挑选与计数随「一键清理」按钮一起退役（#225）。
   *
   * 留着空壳文件会让后人以为"收录行还在前端算失效"——那正是已经删掉的那套。
   */
  it('staleItems.ts 已删除，失效文案只剩 outlineArrange 一处', () => {
    expect(existsSync(join(projectDir, 'app', 'utils', 'staleItems.ts'))).toBe(false)
    expect(adminService).not.toContain('staleItems')
    for (const { file, text } of appSources) {
      expect(text, `${file} 仍引用 staleItems`).not.toContain('staleItems')
    }
  })
})

describe('后台不再有任何路径写旧扁平收录表', () => {
  it('services/admin.ts 不再导出旧收录 / 排序 / 批量接口', () => {
    for (const name of LEGACY_SERVICE_NAMES) {
      expect(adminService, name).not.toContain(`export function ${name}`)
    }
    expect(adminService).toContain('export function getCollectionOutline')
    expect(adminService).toContain('export function saveCollectionOutline')
    expect(adminService).toContain('export function getProjectOutline')
    expect(adminService).toContain('export function saveProjectOutline')
  })

  it('后台 app 里没有页面或工具 import 旧收录 / 排序接口与旧编排工具', () => {
    for (const { file, text } of appSources) {
      for (const name of [...LEGACY_SERVICE_NAMES, ...LEGACY_UTILS]) {
        expect(text, `${file} 仍引用 ${name}`).not.toContain(name)
      }
      expect(text, `${file} 仍提到旧扁平收录表`).not.toMatch(/collection_article_models|project_article_models/u)
    }
  })
})
