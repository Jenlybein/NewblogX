import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const indexVue = read("app/pages/index.vue");
const component = read("app/components/home/HomeInkFluid.vue");
const composable = read("app/composables/useInkFluid.ts");
const utils = read("app/utils/inkFluid.ts");

/**
 * 首屏指针墨流（ink-fluid）的结构契约。
 *
 * 参考组件的 WebGL 模拟是 GPU 行为，happy-dom 里跑不起来；这里锁的是三条边缘：
 *   1. 它只是首屏背景板的装饰层（aria-hidden + pointer-events: none）；
 *   2. 颜色只消费首屏已有的反白 / 功能蓝 token；
 *   3. 没有 WebGL2、或用户要求减少动态时，必须优雅退化而不是报错/继续动。
 * 真实的画布挂载与降级再由组件测试覆盖。
 */
describe("首屏指针墨流", () => {
  it("挂在首屏封面内、画布在内容层之下且点击穿透", () => {
    expect(indexVue).toContain("<HomeInkFluid");
    expect(indexVue).toContain('class="home-cover__ink"');
    expect(indexVue).toContain("HomeInkFluid");
    expect(indexVue).toContain("import HomeInkFluid");

    const ink = indexVue.slice(indexVue.indexOf(".home-cover__ink {"), indexVue.indexOf("}", indexVue.indexOf(".home-cover__ink {")));
    expect(ink).toContain("pointer-events: none");
    expect(ink).toContain("z-index: 1");
  });

  it("组件根节点 aria-hidden、画布不接收事件，且不产生可聚焦控件", () => {
    expect(component).toContain('aria-hidden="true"');
    expect(component).toContain("pointer-events: none");
    expect(component).not.toContain("<button");
    expect(component).not.toContain("tabindex");
  });

  it("颜色只读两档中性反白 token，不写死色值、不引入第五个色相", () => {
    expect(composable).toContain("--color-on-inverse");
    expect(composable).toContain("--color-on-inverse-muted");
    expect(composable).not.toContain("--jelly-interactive");
    expect(composable).not.toMatch(/#[0-9a-f]{3}\b|#[0-9a-f]{6}\b/iu);
    expect(utils).not.toMatch(/#[0-9a-f]{3}\b|#[0-9a-f]{6}\b/iu);
  });

  it("减少动态走固定静帧，WebGL2 不可用走 data-ink-fallback", () => {
    expect(composable).toContain("initialReduced");
    expect(composable).toContain("still()");
    expect(composable).toContain("setReduced");
    expect(composable).toContain("data-ink-fallback");
    expect(composable).toContain("EXT_color_buffer_float");
    expect(composable).toContain("IntersectionObserver");
    expect(composable).toContain("visibilitychange");
  });
});
