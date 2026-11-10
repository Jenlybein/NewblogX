import { describe, expect, it } from 'vitest'

import { applySiteSectionPatch } from '~/utils/siteSectionSave'

/**
 * 站点四页的「读-改-写」合并语义（ADR-0039）。
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
  comment: { skip_examining: boolean }
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
    comment: { skip_examining: true },
    chat: { stranger_msg_enabled: false },
    about: { version: '1.0.0' },
    project: { title: 'JellyBin' },
    reconcile: { interval_minutes: 30 },
    ...overrides,
  }
}

describe('applySiteSectionPatch', () => {
  it('只覆盖本页拥有的块，其它页与无 UI 块原样保留', () => {
    const latest = section({ site_info: { beian: '旧备案号' }, seo: { keywords: 'old', description: 'old' } })
    const patched = section({ site_info: { beian: '新备案号' }, seo: { keywords: 'new', description: 'new' } })

    const merged = applySiteSectionPatch(latest, patched, ['site_info', 'seo'])

    expect(merged.site_info).toEqual({ beian: '新备案号' })
    expect(merged.seo).toEqual({ keywords: 'new', description: 'new' })
    // 兄弟页的字段与无 UI 块一个都不能变（包括对象引用）。
    expect(merged.login).toBe(latest.login)
    expect(merged.article).toBe(latest.article)
    expect(merged.comment).toBe(latest.comment)
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
    const patched = { ...section(), login: undefined as unknown as Section['login'] }

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
