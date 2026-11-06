import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getProjectOutline, saveProjectOutline } from '~/services/admin'

/**
 * #276 后台项目树服务的外部接缝：项目树编辑器只经 #274 的项目具名端点读写
 * `project_outline_node_models`。断言路径 / 方法 / 请求体，不读源码文本。
 */

const requestMock = vi.fn()

beforeEach(() => {
  // 后台没有 #imports 别名：与 imageService.test.ts 同一做法，直接把自动导入的
  // useNuxtApp 塞进全局。
  vi.stubGlobal('useNuxtApp', () => ({ $api: { request: requestMock } }))
  requestMock.mockReset()
  requestMock.mockResolvedValue({ nodes: [] })
})

describe('后台项目树服务：只走项目具名端点', () => {
  it('读回整树：GET /projects/<id>/outline-tree（管理员读面含失效行）', async () => {
    await getProjectOutline('p1')

    expect(requestMock).toHaveBeenCalledWith('/projects/p1/outline-tree')
  })

  it('整树提交：PUT /projects/<id>/outline-tree，节点原样进请求体', async () => {
    const nodes = [{ id: '', kind: 1, title: '第一章', article_id: '', children: [] }]

    await saveProjectOutline('p1', nodes)

    expect(requestMock).toHaveBeenCalledWith('/projects/p1/outline-tree', {
      method: 'PUT',
      body: { nodes },
    })
  })
})
