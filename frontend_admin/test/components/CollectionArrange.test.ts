import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * 旧合集排列组件用例已退役（#218）：那个页面与它的两级排序 / 批量收录接口
 * 一起删除，收录与排序只经「目录编排」的树编辑器。这里保留成门禁，断言
 * 旧页面不会回来；树编辑器的组件契约在 OutlineEditor.test.ts。
 */
describe('CollectionArrange 组件测试已退役', () => {
  it('旧排列页不存在', () => {
    expect(existsSync(join(process.cwd(), 'app/pages/articles/collection-arrange.vue'))).toBe(false)
  })

  it('目录编排页是唯一同时承担新建 / 收录 / 排序 / 重命名 / 移除 / 失效清理的入口', () => {
    const source = readFileSync(join(process.cwd(), 'app/pages/articles/outline.vue'), 'utf8')
    for (const action of ['重命名', '移除', '清理失效', '批量收录', '保存目录树', '上移', '下移', '缩进', '移出', '根层新建文件夹']) {
      expect(source, action).toContain(action)
    }
    expect(source).toContain('saveCollectionOutline')
    expect(source).toContain('saveProjectOutline')
  })
})
