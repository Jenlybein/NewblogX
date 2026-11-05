import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  getCollectionOutline,
  saveCollectionOutline,
} from '~/services/admin'

const requestMock = vi.fn()

/**
 * #275 后台合集树编排的行为接缝：共用编辑器对合集侧改走 #274 的**合集具名端点**
 * `GET|PUT /collections/<id>/outline-tree`。GET 必须带鉴权（具名 GET 是公开读，
 * 管理员例外靠请求里的身份判定：不带令牌会被当成匿名读者，未展示合集读成空树）。
 *
 * 项目侧由 #276 迁移，本票不动它。
 */
describe('collection outline service：合集编辑器走具名端点', () => {
  beforeEach(() => {
    vi.stubGlobal('useNuxtApp', () => ({ $api: { request: requestMock } }))
    requestMock.mockReset()
    requestMock.mockResolvedValue({ nodes: [] })
  })

  it('读回整棵树：GET /collections/<id>/outline-tree（默认带管理员令牌）', async () => {
    await getCollectionOutline('c1')

    expect(requestMock).toHaveBeenCalledWith('/collections/c1/outline-tree')
  })

  it('整树提交：PUT /collections/<id>/outline-tree，body 是嵌套节点', async () => {
    const nodes = [{ kind: 1, title: '第一章', children: [] }]

    await saveCollectionOutline('c1', nodes as never)

    expect(requestMock).toHaveBeenCalledWith('/collections/c1/outline-tree', {
      method: 'PUT',
      body: { nodes },
    })
  })

  it('项目侧函数仍是项目本票之前的共享端点（本票不碰项目分派）', async () => {
    const { getProjectOutline } = await import('~/services/admin')
    await getProjectOutline('p1')

    expect(requestMock).toHaveBeenCalledWith('/projects/p1/outline/admin')
  })
})
