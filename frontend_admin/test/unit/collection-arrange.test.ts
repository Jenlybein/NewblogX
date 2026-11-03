import { existsSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * 旧合集排列页已退役（#218）：收录与排序只经「目录编排」的树编辑器。
 *
 * 本文件保留成门禁而不是删掉：一旦有人把旧排列页 / 旧纯工具放回来，
 * 这里会立刻红，避免"页面又出现第二套顺序真相"被静默合入。
 */
describe('合集排列页已退役', () => {
  it('旧排列页与旧编排纯工具都不存在', () => {
    expect(existsSync(join(process.cwd(), 'app/pages/articles/collection-arrange.vue'))).toBe(false)
    expect(existsSync(join(process.cwd(), 'app/utils/collectionArrange.ts'))).toBe(false)
  })
})
