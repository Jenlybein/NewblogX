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
