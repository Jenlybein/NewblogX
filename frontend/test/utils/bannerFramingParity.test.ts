import { describe, expect, it } from "vitest";

import { bannerFramingStyle } from "~/utils/bannerFraming";
import {
  framingStyle,
  type Framing,
  type Size,
} from "../../../frontend_admin/app/utils/framing";

/**
 * 跨端取景几何等价性（#45 / #46，US9）。
 *
 * 后台取景窗与阅读站各自只有一份"取景参数 → CSS"的纯函数。两端的实现可以各写
 * 一份，但**几何必须等价**：同一组参数、同一张图、同一个 16:9 框，算出的可见
 * 图像区域（在图坐标里）必须逐坐标相同。
 *
 * 这里不从"两边的样式字符串差不多"倒推，而是独立实现一个 CSS 几何求值器：
 * 把 `object-position` / `transform-origin` / `scale` 的语义落成最终图片矩形，
 * 再与框求交、映回原图坐标。两个纯函数的输出分别喂给同一个求值器，比较的是
 * 运营真正能看到的那块图，而不是某条 CSS 的拼写。
 */

interface CssFraming {
  objectPosition?: string | number;
  transformOrigin?: string | number;
  transform?: string | number;
}

interface VisibleRegion {
  /** 原图坐标（像素），不是框坐标。 */
  left: number;
  top: number;
  right: number;
  bottom: number;
}

function fractionOf(percent: string, label: string): number {
  const match = /^(-?\d+(?:\.\d+)?)%$/u.exec(percent.trim());
  if (!match) throw new Error(`${label} 不是百分比：${percent}`);
  return Number(match[1]) / 100;
}

function parsePosition(value: CssFraming["objectPosition"], label: string): { x: number; y: number } {
  if (typeof value !== "string") throw new Error(`${label} 缺失`);
  const parts = value.trim().split(/\s+/u);
  if (parts.length !== 2) throw new Error(`${label} 不是两个分量：${value}`);
  return { x: fractionOf(parts[0]!, `${label} x`), y: fractionOf(parts[1]!, `${label} y`) };
}

function parseScale(value: CssFraming["transform"]): number {
  if (typeof value !== "string") throw new Error("transform 缺失");
  const match = /^scale\((\d+(?:\.\d+)?)\)$/u.exec(value.trim());
  if (!match) throw new Error(`transform 不是单轴 scale：${value}`);
  return Number(match[1]);
}

/**
 * 按 CSS 语义求"最终可见区域"：
 * 1. cover 把原图等比缩到铺满框，得到绘制尺寸；
 * 2. object-position 把图上比例点对到框内同一比例点，得到未缩放的左上角；
 * 3. transform-origin 是以元素框为基准的锚点，scale(zoom) 把图片矩形绕过它撑开；
 * 4. 与 0..frame 求交，再按最终图片宽度映回原图像素。
 */
function visibleRegion(style: CssFraming, image: Size, frame: Size): VisibleRegion {
  const position = parsePosition(style.objectPosition, "objectPosition");
  const origin = parsePosition(style.transformOrigin, "transformOrigin");
  const zoom = parseScale(style.transform);

  const cover = Math.max(frame.width / image.width, frame.height / image.height);
  const drawnWidth = image.width * cover;
  const drawnHeight = image.height * cover;

  // object-position: 0% 贴左/上，100% 贴右/下；中间按 (框 - 图) 的负差插值。
  const offsetX = position.x * (frame.width - drawnWidth);
  const offsetY = position.y * (frame.height - drawnHeight);

  // transform-origin 落在元素框上，不在图片内容上；缩放以它为不动点。
  const originX = origin.x * frame.width;
  const originY = origin.y * frame.height;
  const finalWidth = drawnWidth * zoom;
  const finalHeight = drawnHeight * zoom;
  const finalLeft = originX + (offsetX - originX) * zoom;
  const finalTop = originY + (offsetY - originY) * zoom;

  const visibleLeft = Math.max(0, finalLeft);
  const visibleTop = Math.max(0, finalTop);
  const visibleRight = Math.min(frame.width, finalLeft + finalWidth);
  const visibleBottom = Math.min(frame.height, finalTop + finalHeight);

  return {
    left: ((visibleLeft - finalLeft) / finalWidth) * image.width,
    top: ((visibleTop - finalTop) / finalHeight) * image.height,
    right: ((visibleRight - finalLeft) / finalWidth) * image.width,
    bottom: ((visibleBottom - finalTop) / finalHeight) * image.height,
  };
}

/** 16:9 的参考取景框；比例与真实框一致，所以像素值可直接手算核对。 */
const FRAME: Size = { width: 1600, height: 900 };

const IMAGES: Array<{ name: string; image: Size }> = [
  { name: "超宽横图", image: { width: 3200, height: 900 } },
  { name: "竖图", image: { width: 1080, height: 1920 } },
  { name: "3:2 横图", image: { width: 1200, height: 800 } },
  { name: "像素噪声图", image: { width: 1919, height: 1079 } },
];

const FRAMINGS: Array<{ name: string; framing: Framing }> = [
  { name: "左上角、zoom 1", framing: { focus_x: 0, focus_y: 0, zoom: 1 } },
  { name: "右下角、zoom 1", framing: { focus_x: 1, focus_y: 1, zoom: 1 } },
  { name: "右上角、zoom 1.5", framing: { focus_x: 1, focus_y: 0, zoom: 1.5 } },
  { name: "左下角、zoom 1.5", framing: { focus_x: 0, focus_y: 1, zoom: 1.5 } },
  { name: "偏移焦点、zoom 2", framing: { focus_x: 0.25, focus_y: 0.75, zoom: 2 } },
  // 后端允许任意浮点，UI 步进只是交互粒度：不在 0.05 网格上的存值也必须两端等价。
  { name: "非步进取值", framing: { focus_x: 0.123456, focus_y: 0.987654, zoom: 1.005 } },
];

describe("后台预览与阅读站的可见图像区域逐坐标等价（同一组参数 + 同一张图）", () => {
  for (const { name: imageName, image } of IMAGES) {
    for (const { name: framingName, framing } of FRAMINGS) {
      it(`${imageName} × ${framingName}`, () => {
        const readerRegion = visibleRegion(bannerFramingStyle(framing), image, FRAME);
        const adminRegion = visibleRegion(framingStyle(framing), image, FRAME);

        expect(readerRegion.left).toBeCloseTo(adminRegion.left, 6);
        expect(readerRegion.top).toBeCloseTo(adminRegion.top, 6);
        expect(readerRegion.right).toBeCloseTo(adminRegion.right, 6);
        expect(readerRegion.bottom).toBeCloseTo(adminRegion.bottom, 6);
        // 几何值本身也逐条同形：object-fit / object-position / origin / scale。
        expect(bannerFramingStyle(framing)).toEqual(framingStyle(framing));
      });
    }
  }

  it("同一张竖图放到 16:9 框里：两端都露出画面中段，而不是把整张竖图压扁", () => {
    // 1080×1920 cover 进 1600×900：绘制成 1600×2844.44…，纵向多出 1944.44…，
    // 上下各露一半的溢出。zoom 1、焦点居中时可见区域是整宽、纵向 656.25..1263.75px。
    const framing: Framing = { focus_x: 0.5, focus_y: 0.5, zoom: 1 };
    const region = visibleRegion(bannerFramingStyle(framing), { width: 1080, height: 1920 }, FRAME);

    expect(region.left).toBeCloseTo(0, 6);
    expect(region.right).toBeCloseTo(1080, 6);
    expect(region.top).toBeCloseTo(656.25, 6);
    expect(region.bottom).toBeCloseTo(1263.75, 6);
    // 两端在这组参数下必须得到同一个区域。
    expect(visibleRegion(framingStyle(framing), { width: 1080, height: 1920 }, FRAME)).toEqual(region);
  });
});
