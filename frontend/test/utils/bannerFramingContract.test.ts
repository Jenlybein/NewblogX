import { describe, expect, it } from "vitest";

import { bannerFramingStyle } from "~/utils/bannerFraming";

/**
 * 取景 → CSS 的**契约表**（#46 / #45 / US9）。
 *
 * 为什么是"字面量表"而不是"导入后台的实现再比对"：两个前端之间没有共享代码包
 * （AGENTS.md：唯一耦合是 OpenAPI），而测试 import 另一个 app 的源码会在 CI 里
 * 触发那个 app 的 `.nuxt` 解析（frontend job 并不准备 frontend_admin/.nuxt），
 * 直接炸成 TSConfckParseError。
 *
 * 于是等价性靠**两端都被钉在同一张表上**来保证：后台有一份逐字相同的表
 * （frontend_admin/test/unit/framing-style-contract.test.ts）。任何一端改了取整
 * 或格式化，那一端自己的测试立刻红，不需要跨 app 引用。
 *
 * 表里同时覆盖：缺省、非步进值（检验取整）、角点、上限缩放。
 */
const CASES: { name: string; crop: { focus_x: number; focus_y: number; zoom: number } | undefined }[] = [
  { name: "缺省（历史图）", crop: undefined },
  { name: "居中", crop: { focus_x: 0.5, focus_y: 0.5, zoom: 1 } },
  { name: "偏左上", crop: { focus_x: 0.2, focus_y: 0.15, zoom: 1.5 } },
  { name: "右上角极限", crop: { focus_x: 1, focus_y: 0, zoom: 2 } },
  { name: "非步进值", crop: { focus_x: 0.123456, focus_y: 0.987654, zoom: 1.005 } },
  { name: "非步进缩放", crop: { focus_x: 0.5, focus_y: 0.5, zoom: 1.37 } },
];

const EXPECTED: Record<string, string> = {
  "缺省（历史图）": "50% 50%",
  居中: "50% 50%",
  偏左上: "20% 15%",
  右上角极限: "100% 0%",
  非步进值: "12.35% 98.77%",
  非步进缩放: "50% 50%",
};

const EXPECTED_SCALE: Record<string, string> = {
  "缺省（历史图）": "scale(1)",
  居中: "scale(1)",
  偏左上: "scale(1.5)",
  右上角极限: "scale(2)",
  // 1.005 两位取整回落成 1：这是取整口径，两端一致即可。
  非步进值: "scale(1)",
  非步进缩放: "scale(1.37)",
};

describe("阅读站取景样式＝跨端契约表", () => {
  it.each(CASES)("$name：object/transform-origin 取同一焦点、scale 取缩放", ({ name, crop }) => {
    const style = bannerFramingStyle(crop);

    expect(style.objectFit).toBe("cover");
    expect(style.objectPosition).toBe(EXPECTED[name as keyof typeof EXPECTED]);
    expect(style.transformOrigin).toBe(EXPECTED[name as keyof typeof EXPECTED]);
    expect(style.transform).toBe(EXPECTED_SCALE[name as keyof typeof EXPECTED_SCALE]);
  });

  it("坏数据（NaN / Infinity）退化为缺省，而不是把坏值写进样式", () => {
    const style = bannerFramingStyle({ focus_x: Number.NaN, focus_y: Number.POSITIVE_INFINITY, zoom: 0 });

    expect(style.objectPosition).toBe("50% 50%");
    expect(style.transform).toBe("scale(1)");
  });
});
