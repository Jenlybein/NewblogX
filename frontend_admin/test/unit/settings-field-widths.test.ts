import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

/**
 * 设置面板的宽度口径（决定清单 §7 第 31 条）。
 *
 * 用户的原话是「不要再 100% 宽度铺满全屏」。两层修法：面板级上限（`.admin-settings-panel`
 * 760px + 空白子项 520px）兜住"铺满"，**逐字段语义宽度**再让输入框本身跟内容走——
 * URL 给 560、密钥 480、短键 280、数字 180、下拉 320、长文本 560。
 *
 * 这条门禁只锁一件事：**每个表单控件都显式带了 `.admin-field--*` 之一**。
 * 锁"有没有分类"，不锁"分到哪一类"——后者是审美判断，前者漏一个就又退回 520 一刀切。
 *
 * 两个例外：
 *   - `SettingsStorage` 是最先落地的那一份，同样逐个分类，正常参与检查；
 *   - `SettingsDeco` 的数字输入在自定义的 `.deco-number-input` 壳里（带单位后缀），
 *     宽度由那个壳负责，不受本门禁约束。
 */

const projectDir = process.cwd()
const settingsDir = join(projectDir, 'app/components/settings')

/** 允许豁免的文件（自带宽度容器的面板）。 */
const EXEMPT_FILES = new Set(['SettingsDeco.vue'])

/** 表单控件标签：出现这些就说明这一行/这一块需要一个宽度分类。 */
const CONTROL_TAGS = ['<NInput ', '<NInput\n', '<NInputNumber ', '<NInputNumber\n', '<NSelect ', '<NSelect\n']

/**
 * 取出「设置」页的面板源码。
 *
 * 只认 `Settings*.vue`：同目录下还有 `AiProviderForm.vue`（AI 供应商表单）与
 * `SidebarCardsEditor.vue`（首页卡片编辑器），它们是**别的页面**的控件，各自用
 * Tailwind 宽度工具类（`w-44` / `min-w-48 flex-1`），不归设置面板的口径管。
 */
function readPanels(): Array<{ file: string; source: string }> {
  return readdirSync(settingsDir)
    .filter(name => name.startsWith('Settings') && name.endsWith('.vue') && !EXEMPT_FILES.has(name))
    .map(name => ({ file: name, source: readFileSync(join(settingsDir, name), 'utf8') }))
}

/**
 * 收集一个文件里**所有**表单控件开标签的原文（多行标签一起收）。
 * 只按标签起点切块，块尾取第一个 `>` —— 这些组件都没有内嵌 `>` 的属性值。
 */
function controlTags(source: string): string[] {
  const tags: string[] = []
  for (const marker of CONTROL_TAGS) {
    let from = 0
    for (;;) {
      const start = source.indexOf(marker, from)
      if (start < 0) break
      const end = source.indexOf('>', start)
      tags.push(source.slice(start, end + 1))
      from = start + marker.length
    }
  }
  return tags
}

describe('设置面板：每个表单控件都按语义收宽（不是 520 一刀切）', () => {
  it('所有控件都带了 .admin-field--* 之一', () => {
    const offenders: string[] = []

    for (const { file, source } of readPanels()) {
      for (const tag of controlTags(source)) {
        if (!/admin-field--(?:url|secret|key|number|select|long)/u.test(tag)) {
          offenders.push(`${file}: ${tag.replace(/\s+/gu, ' ').slice(0, 90)}`)
        }
      }
    }

    expect(offenders).toEqual([])
  })

  it('门禁覆盖到了真实控件（不是"没控件所以全绿"）', () => {
    const total = readPanels().reduce((sum, panel) => sum + controlTags(panel.source).length, 0)

    // 六个面板 + Storage，量级在 40 以上；低于这个数说明匹配器失效了。
    expect(total).toBeGreaterThan(40)
  })

  it('判据不是恒绿：把分类去掉必须报红（canary）', () => {
    const sample = '<NInput v-model:value="config.x" placeholder="x" />'
    const classified = '<NInput v-model:value="config.x" class="admin-field--key" />'
    const matcher = /admin-field--(?:url|secret|key|number|select|long)/u

    expect(matcher.test(sample)).toBe(false)
    expect(matcher.test(classified)).toBe(true)
  })
})
