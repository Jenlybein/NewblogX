import { describe, expect, it } from "vitest";

import { framingStyle } from "~/utils/coverFraming";

/**
 * 文章封面取景 → CSS 的**跨端契约表**。
 *
 * 这张表与后台那份**逐字相同**（frontend_admin/test/unit/framing-style-contract.test.ts）。
 * 两个前端之间没有共享代码包（AGENTS.md：唯一耦合是 OpenAPI），所以等价性不靠
 * import 对方，而靠两端都被钉在同一组字面量上：谁改了取整或格式化，谁自己的
 * 测试就红。
 *
 * 读者侧用的就是这一个出口：object-position / transform-origin 取同一个百分比焦点，
 * transform 取缩放；输出里没有任何像素值，所以任意宽度的封面里都是同一块取景。
 */
const CASES: { name: string; framing: { focus_x: number; focus_y: number; zoom: number } }[] = [
  { name: "缺省（历史图）", framing: { focus_x: 0.5, focus_y: 0.5, zoom: 1 } },
  { name: "居中", framing: { focus_x: 0.5, focus_y: 0.5, zoom: 1 } },
  { name: "偏左上", framing: { focus_x: 0.2, focus_y: 0.15, zoom: 1.5 } },
  { name: "右上角极限", framing: { focus_x: 1, focus_y: 0, zoom: 2 } },
  { name: "非步进值", framing: { focus_x: 0.123456, focus_y: 0.987654, zoom: 1.005 } },
  { name: "非步进缩放", framing: { focus_x: 0.5, focus_y: 0.5, zoom: 1.37 } },
];

const EXPECTED: Record<string, string> = {
  '缺省（历史图）': '50% 50%',
  居中: '50% 50%',
  偏左上: '20% 15%',
  右上角极限: '100% 0%',
  非步进值: '12.35% 98.77%',
  非步进缩放: '50% 50%',
};

const EXPECTED_SCALE: Record<string, string> = {
  '缺省（历史图）': 'scale(1)',
  居中: 'scale(1)',
  偏左上: 'scale(1.5)',
  右上角极限: 'scale(2)',
  // 1.005 两位取整回落成 1：这是取整口径，两端一致即可。
  非步进值: 'scale(1)',
  非步进缩放: 'scale(1.37)',
};

describe("文章封面取景样式＝跨端契约表", () => {
  it.each(CASES)("$name：object/transform-origin 取同一焦点、scale 取缩放", ({ name, framing }) => {
    const style = framingStyle(framing);

    expect(style.objectFit).toBe("cover");
    expect(style.objectPosition).toBe(EXPECTED[name]);
    expect(style.transformOrigin).toBe(EXPECTED[name]);
    expect(style.transform).toBe(EXPECTED_SCALE[name]);
  });

  it("存量文章没有取景参数时＝居中、缩放 1（与引入取景前一致）", () => {
    const style = framingStyle(null);

    expect(style.objectPosition).toBe("50% 50%");
    expect(style.transform).toBe("scale(1)");
  });

  it("读者侧按相对百分比复现同一块几何：焦点百分比即图上取景的锚点", () => {
    // 纯函数算出来的相对位置：20% 80% 的焦点 + 1.5 倍缩放。
    // 取景窗、工作室预览与读者站详情页都消费这一份结果，不需要第二套算术。
    const style = framingStyle({ focus_x: 0.2, focus_y: 0.8, zoom: 1.5 });

    expect(style.objectPosition).toBe("20% 80%");
    expect(style.transform).toBe("scale(1.5)");
    expect(style.objectFit).toBe("cover");
  });
});
