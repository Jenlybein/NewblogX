import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 首屏的装饰层必须点击穿透。
 *
 * 色带、细网格、基线、四角裁切标记、记号与刻度都是 aria-hidden 的装饰，压在最上层（z-index 3/4）。
 * 只要其中任何一层没有 `pointer-events: none`，它就会吃掉下面轮播的点击 ——
 * 实测过一次：轮播"下一张"箭头中心命中的是 `li.home-cover__tick`（下沿刻度），
 * 箭头根本点不动，看起来像"轮播坏了"。
 */
const projectDir = process.cwd();
const screenVue = readFileSync(join(projectDir, "app/pages/index.vue"), "utf8");
const carouselVue = readFileSync(
  join(projectDir, "app/components/article/BannerCarousel.vue"),
  "utf8",
);

describe("首屏装饰层点击穿透", () => {
  const layers = [".home-cover__bands", ".home-cover__ink", ".home-cover__aside"] as const;

  for (const layer of layers) {
    it(`${layer} 声明了 pointer-events: none`, () => {
      const block = blockAfter(screenVue, `${layer} {`);
      expect(block, `${layer} 缺少 pointer-events: none`).toContain("pointer-events: none");
    });
  }

  it("装饰层都标了 aria-hidden，不会进入无障碍树", () => {
    const template = screenVue.slice(screenVue.indexOf("<template>"));
    for (const marker of [
      'class="home-cover__bands',
      'class="home-cover__ink',
      'class="home-cover__marks"',
      'class="home-cover__ticks"',
    ]) {
      const at = template.indexOf(marker);
      expect(at, `模板里找不到 ${marker}`).toBeGreaterThan(-1);
      // aria-hidden 与类名在同一层的开标签上（同一行内出现即可）。
      const line = template.slice(at, template.indexOf(">", at));
      expect(line, `${marker} 应当 aria-hidden`).toContain("aria-hidden");
    }
  });

  it("轮播舞台本身是点击穿透的，只有底图与控件接收事件", () => {
    const stage = blockAfter(carouselVue, ".banner-stage {");
    expect(stage).toContain("pointer-events: none");
    // 底图（整张图是链接）与控件区必须显式收回事件。
    expect(blockAfter(carouselVue, ".banner-photo {")).toContain("pointer-events: auto");
    expect(blockAfter(carouselVue, ".banner-controls {")).toContain("pointer-events: auto");
  });

  it("没有给装饰层留 pointer-events: auto 的后门", () => {
    const style = stripComments(screenVue);
    for (const layer of layers) {
      const block = blockAfter(style, `${layer} {`);
      expect(block).not.toContain("pointer-events: auto");
    }
  });
});
