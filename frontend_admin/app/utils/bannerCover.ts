import type { BannerWritePayload } from '~/types/api'

import { DEFAULT_FRAMING, clampFraming, isAspectCompliant } from './framing'
import type { Framing, Size } from './framing'

/**
 * 轮播封面上传前的编排算术（#49）。
 *
 * 为什么把这几步从页面里抽出来：页面里真正会出事的三处都不是渲染，而是
 *   1. 比例不合规的图有没有被拦在**上传之前**；
 *   2. 确认取景后创建轮播时，`crop` 有没有跟着请求走；
 *   3. 上传返回缺 `image_id` 时，会不会仍然创建一条没有封面的轮播。
 * 这三处都写成纯函数或可注入依赖的编排，页面只负责把用户动作接上。
 *
 * 取景算术本身仍在 `~/utils/framing`（本模块只消费它，不复制一套比例逻辑）。
 */

/**
 * 选完文件后的第一问：这张图需要先取景吗？（容差沿用 framing 的 ±1%）
 *
 * 尺寸非法也判"需要取景"：这种图根本没法按 cover 铺满取景框，让它直接上传
 * 只会把问题推到阅读站。
 */
export function requiresFraming(size: Size): boolean {
  return !isAspectCompliant(size.width, size.height)
}

/** 上传器的最小契约：页面把它接到 `uploadImageByTask` 上，测试里换成假实现。 */
export type BannerCoverUploader = (file: File) => Promise<{ image_id?: string; url: string }>

export interface UploadedBannerCover {
  imageId: string
  url: string
}

/**
 * 上传封面，并把可能缺字段的结果收成确定的两个值。
 *
 * 缺少 `image_id` 时直接失败而不是"先创建再说"——否则会留下一条永远没有封面的
 * 轮播，而界面上看起来是成功的。
 */
export async function uploadBannerCover(
  file: File,
  uploader: BannerCoverUploader,
): Promise<UploadedBannerCover> {
  const result = await uploader(file)
  if (!result.image_id || !result.url) {
    throw new Error('上传成功但缺少图片标识')
  }
  return { imageId: result.image_id, url: result.url }
}

/** 创建轮播的入参：取景总是显式带上，缺省即居中、缩放 1。 */
export function buildBannerCreatePayload(input: {
  coverImageId: string
  href: string
  show: boolean
  framing?: Partial<Framing> | null
}): BannerWritePayload {
  return {
    cover_image_id: input.coverImageId,
    href: input.href,
    show: input.show,
    crop: clampFraming(input.framing ?? DEFAULT_FRAMING),
  }
}

/**
 * 浏览器 `Image` 的尺寸探针。
 *
 * 只要求三件浏览器真有的事：赋值 `src`、监听 `load` / `error`、读自然宽高。
 * 抽成接口是为了在 happy-dom 里不真的发图片请求也能覆盖"读到了"和"读不到"两条路径。
 */
export interface ImageSizeProbe {
  naturalWidth: number
  naturalHeight: number
  src: string
  addEventListener(type: 'load' | 'error', listener: () => void): void
}

/**
 * 在浏览器本地读一张图的原始像素尺寸（不经过后端、不先上传）。
 *
 * 读不到（文件损坏、格式不支持、加载失败）返回 `null`——调用方据此报错并中止，
 * 而不是拿 0×0 去判比例。
 */
export function readImageSize(url: string, createProbe: () => ImageSizeProbe): Promise<Size | null> {
  return new Promise((resolve) => {
    const probe = createProbe()

    probe.addEventListener('load', () => {
      const size = { width: probe.naturalWidth, height: probe.naturalHeight }
      resolve(isPositiveSize(size) ? size : null)
    })
    probe.addEventListener('error', () => resolve(null))

    probe.src = url
  })
}

/** 正尺寸（像素宽高必须是有穷的正数）；0×0 视为"没读到"。 */
function isPositiveSize(size: Size): boolean {
  return (
    Number.isFinite(size.width) &&
    Number.isFinite(size.height) &&
    size.width > 0 &&
    size.height > 0
  )
}

/** 列表里的"原图尺寸"：宽 × 高 + 比例是否合规；尺寸读不到时返回 null。 */
export interface BannerCoverSizeView {
  width: number
  height: number
  label: string
  compliant: boolean
}

/**
 * 把尺寸收成列表要显示的两个值。
 *
 * `compliant` 沿用与上传判定同一个 `isAspectCompliant`（±1%），所以列表里的
 * "比例不合规"与上传时是否弹取景窗永远一致，不会出现两套口径。
 */
export function describeBannerCoverSize(
  size: Size | null | undefined,
): BannerCoverSizeView | null {
  if (!size || !isPositiveSize(size)) return null
  return {
    width: size.width,
    height: size.height,
    label: `${size.width} × ${size.height}`,
    compliant: isAspectCompliant(size.width, size.height),
  }
}

/** 重新取景要回填的字段：更新接口是整体替换，所以 href / show 也必须带上。 */
export interface BannerReframeSource {
  id?: string | null
  cover_image_id?: string | null
  href?: string | null
  show?: boolean | null
}

export interface BannerReframeWrite {
  id: string
  payload: BannerWritePayload
}

/**
 * 生成 `PUT /banners/:id` 的请求体。
 *
 * `null` 表示这一行没有封面图片 ID（历史行或数据损坏）：更新接口要求
 * cover_image_id，凭空编一个只会失败，页面据此提示而不是发一个坏请求。
 */
export function buildBannerReframePayload(
  row: BannerReframeSource,
  framing: Partial<Framing> | null,
): BannerReframeWrite | null {
  if (!row.id || !row.cover_image_id) return null
  return {
    id: row.id,
    payload: {
      cover_image_id: row.cover_image_id,
      href: row.href ?? '',
      show: Boolean(row.show),
      crop: clampFraming(framing),
    },
  }
}

/** 更新器的最小契约：页面把它接到 `updateBanner`，测试里换成假实现。 */
export type BannerUpdater = (id: string, payload: BannerWritePayload) => Promise<unknown>

/**
 * 保存一次重新取景。
 *
 * 返回 `false` 表示这一行没有封面 ID、**没有发任何写请求**；抛出则是更新接口
 * 失败。取消路径完全不经过这个函数，因此不会产生写入。
 */
export async function saveBannerReframe(
  row: BannerReframeSource,
  framing: Partial<Framing> | null,
  update: BannerUpdater,
): Promise<boolean> {
  const write = buildBannerReframePayload(row, framing)
  if (!write) return false
  await update(write.id, write.payload)
  return true
}
