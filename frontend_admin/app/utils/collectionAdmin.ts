import type { CollectionCreatePayload, CollectionUpdatePayload } from '~/types/api'

/**
 * 后台合集管理页的编排算术。
 *
 * 为什么抽成纯函数：页面里真正会出事的几处都不是渲染，而是
 *   1. 标题空 / 超长会不会被提交；
 *   2. 没上传封面时列表显示的是不是读者会看到的那张，且两张都没有时不发空 img；
 *   3. 展示开关的请求结果有没有落到列表上。
 * 这些写成纯函数或可注入依赖的编排，页面只负责把用户动作接上。
 *
 * 封面上传在 #200 起改为**上传前定形**（`~/utils/imageShaping` 的 `shapeForUpload`，
 * 与轮播、文章封面同一份实现），所以这里不再有"合规直传 / 不合规先取景"的闸门：
 * 定形之后每张封面都已经是 16:9；#204 起取景参数与取景窗也已全量退场。
 */

/** 标题上限，与后端 `collection.MaxTitleLen` 同一取值。 */
export const TITLE_MAX = 128

/** 描述上限，与后端 `collection.MaxDescriptionLen` 同一取值。 */
export const DESCRIPTION_MAX = 512

/** 表单里的三个可编辑字段（封面是已上传的图片地址）。 */
export interface CollectionFormValues {
  title: string
  description: string
  cover: string
}

/** 表单校验结果；`null` 表示该字段合法。 */
export interface CollectionFormErrors {
  title: string | null
  description: string | null
}

/** 按字符数（不是字节数）量长度：中文一个字算一个。 */
function charCount(value: string): number {
  return Array.from(value).length
}

/** 校验合集表单：标题非空且不超长，描述可空但不超长。 */
export function validateCollectionForm(values: CollectionFormValues): CollectionFormErrors {
  const title = values.title.trim()
  const description = values.description.trim()

  let titleError: string | null = null
  if (!title) {
    titleError = '合集标题不能为空'
  } else if (charCount(title) > TITLE_MAX) {
    titleError = `合集标题不能超过 ${TITLE_MAX} 个字符`
  }

  const descriptionError =
    charCount(description) > DESCRIPTION_MAX ? `合集描述不能超过 ${DESCRIPTION_MAX} 个字符` : null

  return { title: titleError, description: descriptionError }
}

/** 建合集的请求体：字段全部显式给，空封面也要发空串（后端据此回退首篇）。 */
export function buildCollectionCreatePayload(
  values: CollectionFormValues & { displayed: boolean },
): CollectionCreatePayload {
  return {
    title: values.title.trim(),
    description: values.description.trim(),
    cover: values.cover.trim(),
    displayed: values.displayed,
  }
}

/** 改合集的请求体：只提交内容，不带展示开关（免得覆盖别人刚切的状态）。 */
export function buildCollectionUpdatePayload(values: CollectionFormValues): CollectionUpdatePayload {
  return {
    title: values.title.trim(),
    description: values.description.trim(),
    cover: values.cover.trim(),
  }
}

/** 列表 / 表单预览要显示的封面。 */
export interface CollectionCoverView {
  url: string
  /** 这张是回退来的（合集自己没上传封面）。 */
  fallback: boolean
}

/**
 * 解析合集最终要显示的封面。
 *
 * 优先用合集自己上传的封面；没有就回退到首篇收录文章的封面；两张都没有时
 * 返回 `null` —— 调用方必须据此渲染显式占位，**不能**渲染一个 `src=""` 的 img
 * （空 src 会在浏览器里变成指向当前页面的请求，也就是破图）。
 */
export function resolveCollectionCover(
  cover: string | null | undefined,
  fallbackCover: string | null | undefined,
): CollectionCoverView | null {
  const uploaded = (cover ?? '').trim()
  if (uploaded) return { url: uploaded, fallback: false }

  const fallback = (fallbackCover ?? '').trim()
  if (fallback) return { url: fallback, fallback: true }

  return null
}

/** 把一次展示开关的结果落到列表：只改目标行，返回新数组。 */
export function applyDisplayResult<T extends { id: string; displayed: boolean }>(
  rows: T[],
  id: string,
  displayed: boolean,
): T[] {
  return rows.map(row => (row.id === id ? { ...row, displayed } : row))
}

