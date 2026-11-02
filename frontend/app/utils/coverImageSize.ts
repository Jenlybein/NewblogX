import type { Size } from "./coverFraming";

/**
 * 读原图尺寸 + 本地预览地址（只碰浏览器 API，不含业务判断）。
 *
 * 尺寸读不出来时由调用方按"这张图没法判断合规"处理：不弹取景窗、不上传，
 * 直接让用户换一张——没有尺寸就无法保证 16:9 的框里不露白。
 */

/** 尺寸读取器（可注入：单元测试不需要真实的图片解码）。 */
export type CoverImageSizeReader = (file: File) => Promise<Size>;

/** 本地预览地址；环境不支持 object URL 时返回空串（预览留空，不影响上传）。 */
export function createCoverObjectUrl(file: Blob): string {
  try {
    if (typeof URL.createObjectURL === "function") {
      return URL.createObjectURL(file);
    }
  } catch {
    // 非浏览器环境没有 object URL：预览留空即可。
  }
  return "";
}

/** 释放本地预览地址（不释放会一直占着这张图的内存）。 */
export function revokeCoverObjectUrl(url: string) {
  if (!url) return;
  try {
    if (typeof URL.revokeObjectURL === "function") {
      URL.revokeObjectURL(url);
    }
  } catch {
    // 已经释放过或环境不支持：忽略。
  }
}

/** 默认实现：拿本地预览地址喂给 `Image`，读 naturalWidth / naturalHeight。 */
export const readCoverImageSize: CoverImageSizeReader = (file) =>
  new Promise<Size>((resolve, reject) => {
    const url = createCoverObjectUrl(file);
    if (!url) {
      reject(new Error("当前环境无法读取图片尺寸"));
      return;
    }
    const image = new Image();
    image.onload = () => {
      revokeCoverObjectUrl(url);
      resolve({ width: image.naturalWidth, height: image.naturalHeight });
    };
    image.onerror = () => {
      revokeCoverObjectUrl(url);
      reject(new Error("无法解码这张图片"));
    };
    image.src = url;
  });
