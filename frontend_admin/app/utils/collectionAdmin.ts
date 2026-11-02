import type { CollectionCreatePayload, CollectionUpdatePayload } from '~/types/api'
import type { Size } from '~/utils/framing'
import { isAspectCompliant } from '~/utils/framing'

/**
 * 后台合集管理页的编排算术。
 *
 * 为什么抽成纯函数：页面里真正会出事的四处都不是渲染，而是
 *   1. 标题空 / 超长会不会被提交；
 *   2. 比例不合规的封面有没有被拦在**上传之前**；
 *   3. 没上传封面时列表显示的是不是读者会看到的那张，且两张都没有时不发空 img；
 *   4. 展示开关的请求结果有没有落到列表上。
 * 这四处写成纯函数或可注入依赖的编排，页面只负责把用户动作接上。
 *
 * 取景算术本身仍在 `~/utils/framing`，封面上传复用 `~/utils/bannerCover` ——
 * 本模块只消费它们，不复制一套比例逻辑。
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

/** 16:9 闸门的判定结果。 */
export type CoverGate = 'direct' | 'framing' | 'unreadable'

/**
 * 选完封面后的第一问：这张图能直接上传，还是必须先取景？
 *
 * 沿用轮播封面的 ±1% 容差（`isAspectCompliant`），所以两处上传体验不会分叉。
 * 尺寸读不到（0 / NaN / 缺失）返回 `unreadable`：这种图本来就没法按 cover 铺满，
 * 既不该直传，也不该进一个算不出焦点的取景窗。
 */
export function coverUploadGate(size: Size | null | undefined): CoverGate {
  if (!size || !Number.isFinite(size.width) || !Number.isFinite(size.height)) return 'unreadable'
  if (size.width <= 0 || size.height <= 0) return 'unreadable'
  return isAspectCompliant(size.width, size.height) ? 'direct' : 'framing'
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

/** 选封面时页面注入的副作用（测试里换成假实现）。 */
export interface CoverSelectionDeps {
  /** 在本地读原始像素尺寸（不先上传）。 */
  readSize: (url: string) => Promise<Size | null>
  /** 本地预览地址：既是页面预览，也是取景窗的图源。 */
  createPreviewUrl: (file: File) => string
  /** 释放本地预览地址。 */
  revokePreviewUrl: (url: string) => void
  /** 真正上传，返回图片标识与访问地址。 */
  upload: (file: File) => Promise<{ imageId: string; url: string }>
}

/** 选封面的结果：要么已上传，要么进 16:9 取景窗等确认，要么这张图读不出来。 */
export type CoverSelectionResult =
  | { kind: 'uploaded'; cover: string; imageId: string; size: Size }
  | { kind: 'framing'; file: File; previewUrl: string; size: Size }
  | { kind: 'unreadable' }

/**
 * 选封面：先量尺寸，再按 16:9 闸门决定直传还是先取景。
 *
 * 合规图**立即上传**（运营少点一次，上传失败时本地文件还在，可修正后重试）；
 * 不合规图不上传，把文件与原图尺寸交给取景窗，确认后再由页面调用上传。
 */
export async function selectCollectionCover(
  file: File,
  deps: CoverSelectionDeps,
): Promise<CoverSelectionResult> {
  const previewUrl = deps.createPreviewUrl(file)
  const size = await deps.readSize(previewUrl)
  const gate = coverUploadGate(size)

  if (gate === 'unreadable' || !size) {
    deps.revokePreviewUrl(previewUrl)
    return { kind: 'unreadable' }
  }

  if (gate === 'framing') {
    return { kind: 'framing', file, previewUrl, size }
  }

  try {
    const uploaded = await deps.upload(file)
    deps.revokePreviewUrl(previewUrl)
    return { kind: 'uploaded', cover: uploaded.url, imageId: uploaded.imageId, size }
  } catch (error) {
    // 上传失败时本地文件还在（页面据此提示并可重试），预览地址则要释放。
    deps.revokePreviewUrl(previewUrl)
    throw error
  }
}
