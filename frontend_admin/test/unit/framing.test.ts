import { describe, expect, it } from "vitest";

import {
  DEFAULT_FRAMING,
  clampFocus,
  clampFraming,
  clampZoom,
  framedImageRect,
  framingStyle,
  isAspectCompliant,
  nudgeFraming,
  panFraming,
  snapZoom,
  stepFramingZoom,
} from "~/utils/framing";
import type { Size } from "~/utils/framing";

/**
 * 取景算术的单元测试（#45）。
 *
 * 这一组断言全部走**纯函数**：给定输入就得到确定的数或字串，不挂载组件、
 * 不依赖浏览器布局。期望值都是手算出来的独立结论（16:9 的 ±1% 边界、
 * 手推的边界换算），不是把实现里的算式抄一遍。
 */

describe("isAspectCompliant", () => {
  it("标准 16:9 与常见分辨率都合规", () => {
    expect(isAspectCompliant(1920, 1080)).toBe(true);
    expect(isAspectCompliant(1280, 720)).toBe(true);
    expect(isAspectCompliant(3840, 2160)).toBe(true);
  });

  it("像素级噪声（1919×1079 这类）不算不合规", () => {
    // 1919/1079 ≈ 1.77850，与 16/9 差约 0.04%：这是缩图工具/截图留下的噪声，
    // 拿它去要求运营重新取景，只会让"上传即取景"变成每次都弹窗。
    expect(isAspectCompliant(1919, 1079)).toBe(true);
    expect(isAspectCompliant(1921, 1080)).toBe(true);
    expect(isAspectCompliant(1080, 608)).toBe(true);
  });

  it("偏差正好 ±1% 视为合规（闭区间）", () => {
    // 1616×900 → 1616*9/(900*16) = 14544/14400 = 1.01 恰好 +1%；
    // 1584×900 恰好 -1%。边界值必须落在合规一侧，否则"±1%"会退化成"±0.99%"。
    expect(isAspectCompliant(1616, 900)).toBe(true);
    expect(isAspectCompliant(1584, 900)).toBe(true);
  });

  it("超过 1% 判不合规", () => {
    // 1617×900 → +1.0625%；1583×900 → -1.06%。
    expect(isAspectCompliant(1617, 900)).toBe(false);
    expect(isAspectCompliant(1583, 900)).toBe(false);
    // 差得远的：3:2 横图与竖图。
    expect(isAspectCompliant(1200, 800)).toBe(false);
    expect(isAspectCompliant(1080, 1920)).toBe(false);
  });

  it("尺寸缺失或非法时判不合规（不抛错）", () => {
    expect(isAspectCompliant(0, 0)).toBe(false);
    expect(isAspectCompliant(-1920, 1080)).toBe(false);
    expect(isAspectCompliant(1920, 0)).toBe(false);
    expect(isAspectCompliant(Number.NaN, 1080)).toBe(false);
    expect(isAspectCompliant(1920, Number.POSITIVE_INFINITY)).toBe(false);
  });

  it("容差可以显式放宽（判定函数自己不该写死成只有 ±1%）", () => {
    expect(isAspectCompliant(1200, 800, 0.25)).toBe(true);
    expect(isAspectCompliant(1920, 1080, 0)).toBe(true);
    expect(isAspectCompliant(1919, 1079, 0)).toBe(false);
  });
});

describe("clampFocus", () => {
  it("越界贴边，不翻转", () => {
    expect(clampFocus(-0.4)).toBe(0);
    expect(clampFocus(1.7)).toBe(1);
  });

  it("界内的值原样保留（含长小数，四舍五入到 4 位）", () => {
    expect(clampFocus(0)).toBe(0);
    expect(clampFocus(1)).toBe(1);
    expect(clampFocus(0.3333333333)).toBe(0.3333);
  });

  it("非法值回中（0.5），不是回 0", () => {
    // 回 0 会把图顶到左上角——比"居中"糟得多，而且看起来像 bug。
    expect(clampFocus(Number.NaN)).toBe(0.5);
    expect(clampFocus(Number.POSITIVE_INFINITY)).toBe(0.5);
  });
});

describe("clampZoom", () => {
  it("恒在 1–2 之间", () => {
    expect(clampZoom(0.2)).toBe(1);
    expect(clampZoom(9)).toBe(2);
    expect(clampZoom(Number.NaN)).toBe(1);
  });

  it("界内的存值不吸附到步进", () => {
    // 1.37 可能是以后手工调的、或将来接口放宽粒度后的存值：
    // 打开取景窗时把它悄悄改成 1.35，等于"每次打开都改一次线上效果"。
    expect(clampZoom(1.37)).toBe(1.37);
    expect(clampZoom(1)).toBe(1);
  });
});

describe("clampFraming", () => {
  it("三个字段一起钳制", () => {
    expect(clampFraming({ focus_x: -1, focus_y: 2, zoom: 3 })).toEqual({ focus_x: 0, focus_y: 1, zoom: 2 });
  });

  it("缺字段用缺省值（居中、zoom 1）补齐", () => {
    expect(clampFraming({ focus_x: 0.2 })).toEqual({ focus_x: 0.2, focus_y: 0.5, zoom: 1 });
    expect(clampFraming(null)).toEqual(DEFAULT_FRAMING);
    expect(clampFraming(undefined)).toEqual(DEFAULT_FRAMING);
  });
});

describe("snapZoom / stepFramingZoom", () => {
  it("吸附到 0.05 的倍数，且不留下浮点尾巴", () => {
    expect(snapZoom(1.02)).toBe(1);
    expect(snapZoom(1.03)).toBe(1.05);
    expect(snapZoom(1.98)).toBe(2);
    expect(snapZoom(1.1500001)).toBe(1.15);
  });

  it("加减号各走一格，触底触顶就停住", () => {
    expect(stepFramingZoom(DEFAULT_FRAMING, 1).zoom).toBe(1.05);
    expect(stepFramingZoom({ focus_x: 0.5, focus_y: 0.5, zoom: 2 }, 1).zoom).toBe(2);
    expect(stepFramingZoom({ focus_x: 0.5, focus_y: 0.5, zoom: 1 }, -1).zoom).toBe(1);
    expect(stepFramingZoom({ focus_x: 0.5, focus_y: 0.5, zoom: 1.95 }, 1).zoom).toBe(2);
  });

  it("缩放只动 zoom，焦点一点不动", () => {
    const next = stepFramingZoom({ focus_x: 0.2, focus_y: 0.8, zoom: 1 }, 1);
    expect(next.focus_x).toBe(0.2);
    expect(next.focus_y).toBe(0.8);
  });

  it("入参算不出来时不抛错：snapZoom 回下限、step 视为不步进", () => {
    expect(snapZoom(Number.NaN)).toBe(1);
    expect(stepFramingZoom(DEFAULT_FRAMING, Number.NaN).zoom).toBe(1);
  });
});

/** 取景框：800×450 正好是 16:9，方便把"多出多少像素"手算出来。 */
const FRAME: Size = { width: 800, height: 450 };

describe("panFraming", () => {
  it("拖动像素换算成焦点：拖的方向与焦点方向相反", () => {
    // 3200×900 的宽幅图 cover 进 800×450：绘制成 1600×450，横向多出 800px。
    // 把图往右拖 80px（zoom 1）→ 焦点 x 减 80/800 = 0.1。
    const next = panFraming(DEFAULT_FRAMING, 80, 0, FRAME, { width: 3200, height: 900 });
    expect(next.focus_x).toBe(0.4);
  });

  it("放大后同样的手指位移对应更小的焦点变化（光标仍然跟手）", () => {
    // zoom 2：缩放后的横向余量 = 2*1600 − 800 = 2400px，80px → 0.0333。
    const next = panFraming({ ...DEFAULT_FRAMING, zoom: 2 }, 80, 0, FRAME, { width: 3200, height: 900 });
    expect(next.focus_x).toBe(0.4667);
  });

  it("拖到角落就停在角落，越界不外露", () => {
    const left = panFraming(DEFAULT_FRAMING, -9999, 0, FRAME, { width: 3200, height: 900 });
    expect(left.focus_x).toBe(1);
    const right = panFraming(DEFAULT_FRAMING, 9999, 0, FRAME, { width: 3200, height: 900 });
    expect(right.focus_x).toBe(0);
  });

  it("纵向没有可拖的余量时，纵向拖动不改焦点", () => {
    // 3200×900 cover 后纵向正好铺满（余量 0）：纵向拖不动是正确行为，不是失灵。
    const next = panFraming(DEFAULT_FRAMING, 0, 300, FRAME, { width: 3200, height: 900 });
    expect(next.focus_y).toBe(0.5);
  });

  it("竖图只在纵向可拖", () => {
    // 800×1800 cover 进 800×450：绘制 800×1800，纵向多出 1350px。
    const next = panFraming(DEFAULT_FRAMING, 120, -135, FRAME, { width: 800, height: 1800 });
    expect(next.focus_x).toBe(0.5);
    expect(next.focus_y).toBe(0.6);
  });

  it("比例正好 16:9 且没放大时怎么拖都不动", () => {
    expect(panFraming(DEFAULT_FRAMING, 120, -60, FRAME, { width: 1920, height: 1080 })).toEqual(DEFAULT_FRAMING);
  });

  it("尺寸缺失时原样返回（总比算出 NaN 好）", () => {
    expect(panFraming(DEFAULT_FRAMING, 10, 10, FRAME, { width: 0, height: 0 })).toEqual(DEFAULT_FRAMING);
  });

  it("指针坐标算不出来时（NaN）原样返回，不把 NaN 传下去", () => {
    expect(panFraming(DEFAULT_FRAMING, Number.NaN, 0, FRAME, { width: 3200, height: 900 })).toEqual(DEFAULT_FRAMING);
  });
});

describe("nudgeFraming", () => {
  it("方向键把取景范围朝箭头方向挪一格（两轴同一套语义）", () => {
    // 右箭头 = 看右边更多 = 焦点 x 增大；上箭头 = 看上边更多 = 焦点 y 减小。
    // 与鼠标拖动不同向（那是推图），但两轴统一、一句话能讲清：
    // 提示文案写的是"方向键移动取景范围"，不是"移动图片"。
    const next = nudgeFraming(DEFAULT_FRAMING, 1, -1);
    expect(next.focus_x).toBe(0.52);
    expect(next.focus_y).toBe(0.48);
  });

  it("推到边界就停住", () => {
    expect(nudgeFraming(DEFAULT_FRAMING, 999, 0).focus_x).toBe(1);
    expect(nudgeFraming(DEFAULT_FRAMING, -999, 0).focus_x).toBe(0);
    expect(nudgeFraming(DEFAULT_FRAMING, 0, -999).focus_y).toBe(0);
    expect(nudgeFraming(DEFAULT_FRAMING, 0, 999).focus_y).toBe(1);
  });

  it("不碰缩放", () => {
    expect(nudgeFraming({ focus_x: 0.5, focus_y: 0.5, zoom: 1.5 }, 1, 1).zoom).toBe(1.5);
  });

  it("步数非法时按「没按」处理（NaN 不能污染焦点）", () => {
    expect(nudgeFraming(DEFAULT_FRAMING, Number.NaN, Number.NaN)).toEqual(DEFAULT_FRAMING);
  });
});

describe("framedImageRect", () => {
  /** 取景框的四条边：图片的最终矩形必须完全盖住它，否则就是"露白"。 */
  function coversFrame(rect: { left: number; top: number; width: number; height: number }): boolean {
    return (
      rect.left <= 0 &&
      rect.top <= 0 &&
      rect.left + rect.width >= FRAME.width &&
      rect.top + rect.height >= FRAME.height
    );
  }

  it("zoom 1 时露出的是整张图的中间一段（手算值）", () => {
    // 3200×900 cover 进 800×450：绘制 1600×450，横向溢出 800，纵向正好。
    const rect = framedImageRect({ width: 3200, height: 900 }, FRAME, DEFAULT_FRAMING);
    expect(rect).toEqual({ left: -400, top: 0, width: 1600, height: 450 });
  });

  it("放大是把矩形以焦点为锚点撑开", () => {
    // focus 0.25、zoom 2：锚点 x = 200，左边 −200 距锚点 −400，撑成 −800 → 屏幕左 −600，宽 3200；
    // 纵向锚点 y = 112.5，上边 0 距锚点 −112.5，撑成 −225 → 屏幕顶 −112.5，高 900。
    const rect = framedImageRect({ width: 3200, height: 900 }, FRAME, { focus_x: 0.25, focus_y: 0.25, zoom: 2 });
    expect(rect.left).toBe(-600);
    expect(rect.top).toBe(-112.5);
    expect(rect.width).toBe(3200);
    expect(rect.height).toBe(900);
  });

  it("任何合法取景参数都不会露出空白（角点 + 极端比例扫一遍）", () => {
    const images: Size[] = [
      { width: 3200, height: 900 }, // 超宽
      { width: 800, height: 1800 }, // 竖图
      { width: 1920, height: 1080 }, // 正好 16:9
      { width: 1919, height: 1079 }, // 噪声
      { width: 400, height: 400 }, // 方图
    ];
    for (const image of images) {
      for (const focus of [0, 0.25, 0.5, 0.75, 1]) {
        for (const zoom of [1, 1.05, 1.5, 2]) {
          const rect = framedImageRect(image, FRAME, { focus_x: focus, focus_y: focus, zoom });
          expect(coversFrame(rect), `image=${image.width}×${image.height} focus=${focus} zoom=${zoom}`).toBe(true);
        }
      }
    }
  });

  it("尺寸非法时给出零矩形（组件据此降级，不拿 NaN 去算样式）", () => {
    expect(framedImageRect({ width: 0, height: 0 }, FRAME, DEFAULT_FRAMING)).toEqual({
      left: 0,
      top: 0,
      width: 0,
      height: 0,
    });
  });
});

describe("framingStyle", () => {
  it("缺省取景：居中、不放大", () => {
    expect(framingStyle(DEFAULT_FRAMING)).toEqual({
      objectFit: "cover",
      objectPosition: "50% 50%",
      transformOrigin: "50% 50%",
      transform: "scale(1)",
    });
  });

  it("焦点同时决定 object-position 与 transform-origin（同一锚点）", () => {
    // 两者必须是同一个点：object-position 把图上这一点对到框内这一点，
    // transform-origin 让放大也围着这一点，于是"焦点不动、图变大"。
    const style = framingStyle({ focus_x: 0.25, focus_y: 0.75, zoom: 1.5 });
    expect(style.objectPosition).toBe("25% 75%");
    expect(style.transformOrigin).toBe("25% 75%");
    expect(style.transform).toBe("scale(1.5)");
  });

  it("只输出 cover、百分比与 scale，不含像素（预览才能与任意尺寸的最终渲染同一套几何）", () => {
    const style = framingStyle({ focus_x: 0.3333333, focus_y: 0.6666666, zoom: 2 });
    const serialized = Object.values(style).join(" ");
    expect(serialized).not.toMatch(/px/u);
    expect(serialized).toBe("cover 33.33% 66.67% 33.33% 66.67% scale(2)");
  });

  it("非法/越界参数先钳制再出样式", () => {
    expect(framingStyle({ focus_x: -3, focus_y: 9, zoom: 6 })).toEqual({
      objectFit: "cover",
      objectPosition: "0% 100%",
      transformOrigin: "0% 100%",
      transform: "scale(2)",
    });
  });
});
