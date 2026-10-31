import { describe, expect, it } from "vitest";

import {
  CHARM_LOGO_SIZE_DEFAULT,
  CHARM_LOGO_SIZE_MAX,
  CHARM_LOGO_SIZE_MIN,
  CHARM_TETHER_LENGTH_PX,
  charmLayerEnabled,
  charmLogoCenter,
  charmTetherPath,
  clampCharmLogoSize,
} from "~/utils/decoCharm";

/**
 * 为什么值得测：这段决定"挂件出不出现、出在哪个像素上、线画到哪"。
 * 全是纯函数，但每一个边界都有可见后果 ——
 * 门控算错会让手机用户在滑动时看到一根跟着手指的绳子；
 * 坐标算错会把站标画到视口外，这时页面看起来就是"挂件没了"。
 */

describe("光标挂件的参数常量", () => {
  it("挂绳长度本票是模块常量 120px（票 #41 才换成配置参数）", () => {
    // 钉住取值本身：这条常量一旦被"临时"改成配置读取，就说明越权做了 #41 的范围。
    expect(CHARM_TETHER_LENGTH_PX).toBe(120);
  });

  it("站标尺寸区间与默认值与后端一致（24–96，默认 48）", () => {
    expect(CHARM_LOGO_SIZE_MIN).toBe(24);
    expect(CHARM_LOGO_SIZE_MAX).toBe(96);
    expect(CHARM_LOGO_SIZE_DEFAULT).toBe(48);
  });
});

describe("clampCharmLogoSize", () => {
  it("越界钳到边界（不产生发散值）", () => {
    expect(clampCharmLogoSize(4000)).toBe(CHARM_LOGO_SIZE_MAX);
    expect(clampCharmLogoSize(10)).toBe(CHARM_LOGO_SIZE_MIN);
    expect(clampCharmLogoSize(96.5)).toBe(CHARM_LOGO_SIZE_MAX);
  });

  it("区间内的值原样保留，边界值不漂移", () => {
    expect(clampCharmLogoSize(60)).toBe(60);
    expect(clampCharmLogoSize(24)).toBe(24);
    expect(clampCharmLogoSize(96)).toBe(96);
  });

  it("缺省（0）与非法值（负数 / NaN / Infinity）回落默认值", () => {
    // 0 是"字段没填"的零值，不是"我想调到最小"的意图。
    expect(clampCharmLogoSize(0)).toBe(CHARM_LOGO_SIZE_DEFAULT);
    expect(clampCharmLogoSize(-5)).toBe(CHARM_LOGO_SIZE_DEFAULT);
    expect(clampCharmLogoSize(Number.NaN)).toBe(CHARM_LOGO_SIZE_DEFAULT);
    expect(clampCharmLogoSize(Number.POSITIVE_INFINITY)).toBe(CHARM_LOGO_SIZE_DEFAULT);
  });
});

describe("charmLayerEnabled", () => {
  const base = { finePointerHover: true, prefersReducedMotion: false, configEnabled: true };

  it("三条门控同时满足才启用（精确指针 + 未减少动态 + 总开关）", () => {
    expect(charmLayerEnabled(base)).toBe(true);
  });

  it("触屏 / 无精确指针时不启用", () => {
    expect(charmLayerEnabled({ ...base, finePointerHover: false })).toBe(false);
  });

  it("系统开启「减少动态」时不启用（是整站不出现，不是放慢）", () => {
    expect(charmLayerEnabled({ ...base, prefersReducedMotion: true })).toBe(false);
  });

  it("后台关掉总开关时不启用", () => {
    expect(charmLayerEnabled({ ...base, configEnabled: false })).toBe(false);
  });
});

describe("charmLogoCenter", () => {
  it("站标固定在指针正下方一个绳长处（本票不做物理）", () => {
    expect(charmLogoCenter({ x: 100, y: 200 })).toEqual({ x: 100, y: 200 + CHARM_TETHER_LENGTH_PX });
  });

  it("允许显式指定绳长（票 #41 接配置参数时不用改函数签名）", () => {
    expect(charmLogoCenter({ x: 0, y: 0 }, 40)).toEqual({ x: 0, y: 40 });
  });

  it("非有限坐标按 0 处理，站标仍落在绳长处（SVG 里 NaN 会让整条路径消失）", () => {
    expect(charmLogoCenter({ x: Number.NaN, y: Number.POSITIVE_INFINITY })).toEqual({
      x: 0,
      y: CHARM_TETHER_LENGTH_PX,
    });
  });
});

describe("charmTetherPath", () => {
  it("从指针画一条直线到站标（无曲线、无物理）", () => {
    expect(charmTetherPath({ x: 100, y: 200 }, { x: 100, y: 320 })).toBe("M100 200L100 320");
  });

  it("坐标保留一位小数，避免长浮点写进 d 属性", () => {
    expect(charmTetherPath({ x: 10.123456, y: 20.987654 }, { x: 30.44, y: 40.26 })).toBe(
      "M10.1 21L30.4 40.3",
    );
  });

  it("非有限坐标收敛到 0，不产出 NaN 路径", () => {
    expect(charmTetherPath({ x: Number.NaN, y: 0 }, { x: 10, y: Number.NaN })).toBe("M0 0L10 0");
  });
});
