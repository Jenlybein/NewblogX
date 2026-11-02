import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * 后台外壳与面包屑的接线。
 *
 * 后台外壳依赖一整串 Nuxt 自动导入（路由、运行时配置、auth store、naive-ui 消息），
 * 单测挂不起来；这里读真实文件做结构断言 —— 拦的是"改了一半"：
 * 面包屑组件写好了但内容区顶部没挂，或者那句所有页面都一样的固定副标题还留着，
 * 与面包屑的层级语义重复。
 */
const shellVue = readFileSync(
  join(process.cwd(), 'app/components/admin/AdminShell.vue'),
  'utf8',
)

describe('后台外壳面包屑接线', () => {
  it('内容区顶部渲染面包屑，且数据来自菜单层级（buildAdminBreadcrumb）', () => {
    expect(shellVue).toContain('<AdminBreadcrumb')
    expect(shellVue).toContain('buildAdminBreadcrumb')
    // 面包屑在页面标题之上（内容区顶部）。
    expect(shellVue.indexOf('<AdminBreadcrumb')).toBeLessThan(
      shellVue.indexOf('admin-page-title'),
    )
  })

  it('所有页面相同的固定副标题不再与面包屑重复', () => {
    expect(shellVue).not.toContain('基于 OpenAPI 契约接入后台管理能力')
  })
})
