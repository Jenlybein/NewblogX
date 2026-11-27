import { describe, expect, it } from 'vitest'

import { applySiteBlockFieldPatch, applySiteSectionPatch } from '~/utils/siteSectionSave'

/**
 * 站点四页的「读-改-写」合并语义（ADR-0029）。
 *
 * 这是拆页唯一的安全写法：每页只能覆盖自己拥有的顶层块，其它块（兄弟页字段、
 * 以及后台没有 UI 的 about / project / reconcile）必须原样往返。
 *
 * 断言的是**可观察结果**（合并后的对象长什么样），不关心实现细节。
 */

type Section = {
  site_info: { beian: string }
  seo: { keywords: string; description: string }
  login: { email_login: boolean }
  article: { skip_examining: boolean }
  chat: { stranger_msg_enabled: boolean }
  // 后台没有 UI、必须原样保留的三块。
  about: { version: string }
  project: { title: string }
  reconcile: { interval_minutes: number }
}

function section(overrides: Partial<Section> = {}): Section {
  return {
    site_info: { beian: '京ICP备 0001 号' },
    seo: { keywords: '', description: '' },
    login: { email_login: false },
    article: { skip_examining: true },
    chat: { stranger_msg_enabled: false },
    about: { version: '1.0.0' },
    project: { title: 'JellyBin' },
    reconcile: { interval_minutes: 30 },
    ...overrides,
  }
}

describe('applySiteSectionPatch', () => {
  it('只覆盖本页拥有的块，其它页与无 UI 块原样保留', () => {
    const latest = section({
      site_info: { beian: '旧备案号' },
      seo: { keywords: 'old', description: 'old' },
    })
    const patched = section({
      site_info: { beian: '新备案号' },
      seo: { keywords: 'new', description: 'new' },
    })

    const merged = applySiteSectionPatch(latest, patched, ['site_info', 'seo'])

    expect(merged.site_info).toEqual({ beian: '新备案号' })
    expect(merged.seo).toEqual({ keywords: 'new', description: 'new' })
    // 兄弟页的字段与无 UI 块一个都不能变（包括对象引用）。
    expect(merged.login).toBe(latest.login)
    expect(merged.article).toBe(latest.article)
    expect(merged.chat).toBe(latest.chat)
    expect(merged.about).toBe(latest.about)
    expect(merged.project).toBe(latest.project)
    expect(merged.reconcile).toBe(latest.reconcile)
  })

  it('保存另一页时不会把前一页刚存的字段覆盖回去', () => {
    // A 页（站点基础）刚保存了备案号；B 页（私信）加载的是保存前的旧快照。
    const staleSnapshot = section({ site_info: { beian: '旧备案号' } })
    const fresh = section({ site_info: { beian: '新备案号' } })

    const merged = applySiteSectionPatch(fresh, staleSnapshot, ['chat'])
    expect(merged.site_info).toEqual({ beian: '新备案号' })
    expect(merged.chat).toBe(staleSnapshot.chat)
  })

  it('patch 里缺该块（undefined）时不写入，避免把旧数据清空', () => {
    const latest = section()
    const patched = {
      ...section(),
      login: undefined as unknown as Section['login'],
    }

    const merged = applySiteSectionPatch(latest, patched, ['login'])
    expect(merged.login).toBe(latest.login)
  })

  it('不原地修改入参', () => {
    const latest = section()
    const patched = section({ chat: { stranger_msg_enabled: true } })
    const merged = applySiteSectionPatch(latest, patched, ['chat'])

    expect(merged).not.toBe(latest)
    expect(latest.chat).toEqual({ stranger_msg_enabled: false })
    expect(patched.chat).toEqual({ stranger_msg_enabled: true })
  })
})
/**
 * 字段级补丁：site 段的 article 块被两页共用（2026-11-27 拆分）。
 *
 * 块级替换会让后保存的那页把兄弟页刚改的字段写回旧值，所以拆页时必须
 * 声明"本页只拥有这几个字段"。
 */
type SharedSection = {
  article: {
    allow_non_admin_publish: boolean
    skip_examining: boolean
    submit_daily_max: number
    quality_gate: { pass_score: number }
  }
  about: { version: string }
}

function sharedSection(overrides: Partial<SharedSection['article']> = {}): SharedSection {
  return {
    article: {
      allow_non_admin_publish: true,
      skip_examining: false,
      submit_daily_max: 15,
      quality_gate: { pass_score: 50 },
      ...overrides,
    },
    about: { version: '1.0.0' },
  }
}

describe('applySiteBlockFieldPatch', () => {
  const PUBLISH_FIELDS = ['allow_non_admin_publish'] as const
  const REVIEW_FIELDS = ['skip_examining', 'submit_daily_max', 'quality_gate'] as const

  it('只覆盖本页拥有的字段，兄弟页字段与其它块原样保留', () => {
    // 发布页加载时的快照（审核设置页已经改过免审核开关，发布页不知道）。
    const stalePublishSnapshot = sharedSection({
      skip_examining: false,
      submit_daily_max: 15,
    })
    const latest = sharedSection({
      skip_examining: true,
      submit_daily_max: 30,
    })

    const merged = applySiteBlockFieldPatch(latest, stalePublishSnapshot, 'article', PUBLISH_FIELDS)

    expect(merged.article.allow_non_admin_publish).toBe(true)
    // 兄弟页（审核设置）刚改的两项必须活下来。
    expect(merged.article.skip_examining).toBe(true)
    expect(merged.article.submit_daily_max).toBe(30)
    expect(merged.about).toBe(latest.about)
  })

  it('审核设置页保存时不会覆盖发布页的开关', () => {
    const staleReviewSnapshot = sharedSection({
      allow_non_admin_publish: true,
    })
    const latest = sharedSection({
      allow_non_admin_publish: false,
      quality_gate: { pass_score: 60 },
    })

    const merged = applySiteBlockFieldPatch(latest, staleReviewSnapshot, 'article', REVIEW_FIELDS)

    // 发布页拥有的字段（不在本页覆盖清单里）保住最新值。
    expect(merged.article.allow_non_admin_publish).toBe(false)
    // 审核设置页拥有的字段按本次提交覆盖（quality_gate 归审核页所有）。
    expect(merged.article.quality_gate).toEqual({ pass_score: 50 })
  })

  it('字段为 undefined 时不写入；不原地修改入参', () => {
    const latest = sharedSection({ submit_daily_max: 20 })
    const patched = sharedSection()
    delete (patched.article as Partial<SharedSection['article']>).submit_daily_max

    const merged = applySiteBlockFieldPatch(latest, patched, 'article', REVIEW_FIELDS)
    expect(merged.article.submit_daily_max).toBe(20)
    expect(latest.article.submit_daily_max).toBe(20)
    expect(patched.article.skip_examining).toBe(false)
  })

  it('latest 缺该块时按空对象起底，不炸', () => {
    const latest = { about: { version: '1.0.0' } } as SharedSection
    const merged = applySiteBlockFieldPatch(latest, sharedSection(), 'article', PUBLISH_FIELDS)
    expect(merged.article.allow_non_admin_publish).toBe(true)
  })
})
