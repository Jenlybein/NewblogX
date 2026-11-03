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
  it('两个排列页文件不存在，目录编排页仍在', () => {
    expect(existsSync(join(articlesDir, 'collection-arrange.vue'))).toBe(false)
    expect(existsSync(join(articlesDir, 'project-arrange.vue'))).toBe(false)
    expect(existsSync(join(articlesDir, 'outline.vue'))).toBe(true)
  })

  it('合集 / 项目列表页不再有「收录管理」弹窗', () => {
    for (const page of ['collections.vue', 'projects.vue']) {
      const source = withoutComments(readFileSync(join(articlesDir, page), 'utf8'))
      expect(source, page).not.toContain('收录管理')
      expect(source, page).not.toContain('NModal')
      expect(source, page).not.toContain('staleItems')
    }
  })

  it('菜单没有旧排列入口，目录编排是「文章管理」组里唯一的编排入口', () => {
    const paths = ADMIN_NAV.map(item => item.path)
    expect(paths).not.toContain('/articles/collection-arrange')
    expect(paths).not.toContain('/articles/project-arrange')
    expect(paths.filter(path => path.startsWith('/articles/'))).toEqual([
      '/articles/hot-top',
      '/articles/collections',
      '/articles/projects',
      '/articles/outline',
    ])
    const articleGroup = ADMIN_NAV_TREE.find(node => node.label === '文章管理')!
    expect(articleGroup.children!.filter(child => child.label.includes('编排')).map(child => child.label)).toEqual(['目录编排'])
    expect(FLAT_NAV.filter(item => item.label.includes('编排')).map(item => item.label)).toEqual(['目录编排'])
  })

  it('页面数量硬断言：25 个页面退役两个排列页后剩 23 个', () => {
    const pages: string[] = []
    const walk = (dir: string) => {
      for (const entry of readdirSync(dir, { withFileTypes: true })) {
        const path = join(dir, entry.name)
        if (entry.isDirectory()) walk(path)
        else if (entry.name.endsWith('.vue')) pages.push(path)
      }
    }
    walk(pagesDir)
    expect(pages.length).toBe(23)
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
