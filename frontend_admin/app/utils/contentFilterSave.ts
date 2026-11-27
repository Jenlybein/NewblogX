/**
 * content_filter 段的保存归属（纯函数，两页共用）。
 *
 * 事故背景：词表与白名单拆成两页之后，两页仍然读写**同一个**整段端点
 * `PUT /site/content_filter`。后端把请求体 `SiteSectionContentFilterRequest`
 * 整体转换成 `conf.ContentFilter`（缺字段就是零值），**不是**逐字段合并。
 * 所以任意一页只提交自己那几项，另一页的字段就会被清空 —— 在词表页点一次
 * 「保存保留期」就会把白名单清掉，而且保存成功、界面不报错。
 *
 * 正确写法与站点段拆页（utils/siteSectionSave.ts）同一条：**取最新整段 →
 * 只覆盖本页拥有的字段 → 写回**。本文件只负责中间那一步的纯计算，方便单测；
 * 取与写由组件自己调服务层。
 */

import type {
  ContentFilterRuntimeConfig,
  ContentFilterRuntimeConfigPayload,
  ContentFilterWord,
} from "~/types/api"

/** content_filter 段里由 UI 拥有的四个字段（响应里的 required_* / whitelist_stats 是只读派生）。 */
export type ContentFilterField = "words" | "retain_days" | "whitelist" | "whitelist_phrases"

/** 词表页拥有：词表 + 保留期。 */
export const CONTENT_FILTER_WORD_FIELDS: readonly ContentFilterField[] = ["words", "retain_days"]

/** 白名单页拥有：两份白名单。 */
export const CONTENT_FILTER_WHITELIST_FIELDS: readonly ContentFilterField[] = [
  "whitelist",
  "whitelist_phrases",
]

/**
 * 用本页的字段覆盖 latest 的对应字段，其余字段原样带回（返回新对象）。
 *
 * 三条规则：
 *   - `owned` 之外的字段一律取 `latest`：兄弟页刚存的改动不能被旧快照覆盖；
 *   - `owned` 里的字段若本页没给（`undefined`），退回 `latest` 的值；
 *   - 不原地修改入参：组件可能还持有 latest 做展示。
 *
 * 注意返回的是**完整请求体**：请求契约的四个字段都是必填（缺一个后端就按零值覆盖），
 * 所以这里永远构造四个字段，而不是只构造"本页要改的那几个"。
 *
 * 后端把 nil 切片序列化成 null，老库的段可能字段不全，所以这里对数组/数字兜一层默认值。
 */
export function mergeContentFilterPayload(
  latest: ContentFilterRuntimeConfig,
  mine: Partial<ContentFilterRuntimeConfigPayload>,
  owned: readonly ContentFilterField[],
): ContentFilterRuntimeConfigPayload {
  const pickWords = (): ContentFilterWord[] =>
    owned.includes("words") && mine.words !== undefined ? mine.words : latest.words ?? []
  const pickRetainDays = (): number =>
    owned.includes("retain_days") && mine.retain_days !== undefined
      ? mine.retain_days
      : latest.retain_days ?? 90
  const pickWhitelist = (): string[] =>
    owned.includes("whitelist") && mine.whitelist !== undefined
      ? mine.whitelist
      : latest.whitelist ?? []
  const pickWhitelistPhrases = (): string[] =>
    owned.includes("whitelist_phrases") && mine.whitelist_phrases !== undefined
      ? mine.whitelist_phrases
      : latest.whitelist_phrases ?? []

  return {
    words: pickWords(),
    retain_days: pickRetainDays(),
    whitelist: pickWhitelist(),
    whitelist_phrases: pickWhitelistPhrases(),
  }
}

/**
 * 词表的未保存改动数：新增 + 删除 + 档位/类目被改动的条目数。
 *
 * 为什么按"条"而不是按"字段"：脏标记要回答的是"还有几条没落库"，
 * 一条词同时改了档位和类目也只算一条，运营不会因为改了两个字就数出两条。
 */
export function countWordTableChanges(
  baseline: readonly ContentFilterWord[],
  next: readonly ContentFilterWord[],
): number {
  const before = new Map(baseline.map((entry) => [entry.word.toLowerCase(), entry]))
  const after = new Map(next.map((entry) => [entry.word.toLowerCase(), entry]))

  let changes = 0
  for (const [key, entry] of after) {
    const previous = before.get(key)
    if (!previous || previous.tier !== entry.tier || previous.category !== entry.category) {
      changes += 1
    }
  }
  for (const key of before.keys()) {
    if (!after.has(key)) changes += 1
  }
  return changes
}

/** 白名单（词/短语）的未保存改动数：新增 + 删除条数（大小写不敏感去重）。 */
export function countTagChanges(
  baseline: readonly string[] | undefined,
  next: readonly string[],
): number {
  const before = new Set((baseline ?? []).map((item) => item.toLowerCase()))
  const after = new Set(next.map((item) => item.toLowerCase()))

  let changes = 0
  for (const key of after) {
    if (!before.has(key)) changes += 1
  }
  for (const key of before) {
    if (!after.has(key)) changes += 1
  }
  return changes
}
