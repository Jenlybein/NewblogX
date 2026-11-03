import { existsSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * 旧项目排列页已退役（#218）：收录与排序只经「目录编排」的树编辑器。
 * 与合集侧同一条门禁，保留文件而不是删掉，防止旧入口回流。
 */
describe('项目排列页已退役', () => {
  it('旧排列页与旧编排纯工具都不存在', () => {
    expect(existsSync(join(process.cwd(), 'app/pages/articles/project-arrange.vue'))).toBe(false)
    expect(existsSync(join(process.cwd(), 'app/utils/projectArrange.ts'))).toBe(false)
  })
})
