import { readFileSync } from "node:fs";
import { join } from "node:path";

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ImageCropView from "~/components/studio/ImageCropView.vue";
import {
  COVER_SHAPE_RATIO_TEXT,
  COVER_SHAPE_TARGET,
  DEFAULT_SHAPE_FRAMING,
  SHAPE_FRAMING_ZOOM_MAX,
} from "~/utils/imageShaping";

/**
 * 裁剪窗的交互面（#215）：固定比例框 + 拖动画面 + 1–3× 缩放，键盘可达、Esc 取消。
 *
 * 这里只测**用户能做的事**与它产生的取景状态；"选区怎么烤进文件"由
 * `imageShaping` 的纯函数与 `useImageCropUpload` 的用例负责。
 */

function mountView(props: Record<string, unknown> = {}) {
  return mount(ImageCropView, {
    props: {
      src: "blob:original",
      imageWidth: 2000,
      imageHeight: 900,
      target: COVER_SHAPE_TARGET,
      ratioText: COVER_SHAPE_RATIO_TEXT,
      ...props,
    },
  });
}

function frame(wrapper: ReturnType<typeof mountView>) {
  return wrapper.get('[data-role="crop-frame"]');
}

async function confirmByButton(wrapper: ReturnType<typeof mountView>) {
  await wrapper.get('[aria-label="确认裁剪"]').trigger("click");
  return (wrapper.emitted("confirm")?.at(-1)?.[0] ?? null) as { zoom: number; centerX: number; centerY: number } | null;
}

describe("ImageCropView · 固定比例框", () => {
  /** 读框上的比例声明（源码里的比例来自目标尺寸，这里按比例值比较）。 */
  function frameRatio(wrapper: ReturnType<typeof mountView>): number {
    const style = frame(wrapper).attributes("style") ?? "";
    const value = (/aspect-ratio:\s*([^;]+);/u.exec(style)?.[1] ?? "").trim();
    const [width, height] = value.split("/").map((part) => Number(part.trim()));
    expect(Number.isFinite(width) && Number.isFinite(height) && height > 0).toBe(true);
    return width / height;
  }

  it("框的比例来自目标（封面 16:9），不是组件里另写一个数字", () => {
    const wrapper = mountView();
    const frameEl = frame(wrapper);

    expect(frameRatio(wrapper)).toBeCloseTo(COVER_SHAPE_TARGET.width / COVER_SHAPE_TARGET.height, 6);
    // 独立来源：规格写的是 16:9（不是拿 1600/900 自己验自己）。
    expect(frameRatio(wrapper)).toBeCloseTo(16 / 9, 6);
    expect(frameEl.attributes("style")).toContain("aspect-ratio");
    // 头像入口用同一组件：比例跟着目标走（1:1）。
    const avatar = mountView({ target: { width: 256, height: 256 }, ratioText: "1:1" });
    expect(frameRatio(avatar)).toBe(1);
  });

  it("框是键盘可达的，且可访问名称说清了操作", () => {
    const frameEl = frame(mountView());

    expect(frameEl.attributes("tabindex")).toBe("0");
    expect(frameEl.attributes("role")).toBe("group");
    expect(frameEl.attributes("aria-label")).toContain("16:9");
    expect(frameEl.attributes("aria-label")).toContain("方向键");
  });

  it("预览画面由源矩形反推：与最终烤进文件的是同一组数", () => {
    const wrapper = mountView();
    const image = wrapper.get('[data-role="crop-image"]');

    // 2000×900 取 16:9：可视区 1600×900、左边裁掉 200 → 图宽 125%、左移 12.5%。
    expect(image.attributes("style")).toContain("left: -12.5%");
    expect(image.attributes("style")).toContain("width: 125%");
    expect(image.attributes("style")).toContain("height: 100%");
  });
});

describe("ImageCropView · 拖动画面", () => {
  it("拖动画面的距离换算成取景状态，确认时带上它（不是只挪动 CSS）", async () => {
    const wrapper = mountView();
    const frameEl = frame(wrapper);

    // happy-dom 没有布局引擎：组件退回 800px 宽的参考框（换算的是比例，与框多大无关）。
    await frameEl.trigger("pointerdown", { clientX: 100, clientY: 50, button: 0 });
    window.dispatchEvent(new MouseEvent("pointermove", { clientX: 180, clientY: 50 }));
    await wrapper.vm.$nextTick();
    window.dispatchEvent(new MouseEvent("pointerup", { clientX: 180, clientY: 50 }));
    await wrapper.vm.$nextTick();

    // 拖了 80/800 = 10% 视口宽：取景中心左移 0.08。
    const framing = await confirmByButton(wrapper);
    expect(framing?.centerX).toBeCloseTo(0.42, 6);
  });

  it("拖动画面结束后不再跟着指针走（监听器已摘掉）", async () => {
    const wrapper = mountView();
    const frameEl = frame(wrapper);

    await frameEl.trigger("pointerdown", { clientX: 100, clientY: 50, button: 0 });
    window.dispatchEvent(new MouseEvent("pointerup", { clientX: 100, clientY: 50 }));
    await wrapper.vm.$nextTick();
    window.dispatchEvent(new MouseEvent("pointermove", { clientX: 300, clientY: 50 }));
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("update:framing")).toBeUndefined();
  });
});

describe("ImageCropView · 键盘可达与缩放", () => {
  it("方向键移动取景范围：右箭头让视口向右", async () => {
    const wrapper = mountView();
    await frame(wrapper).trigger("keydown", { key: "ArrowRight" });

    const framing = await confirmByButton(wrapper);
    // 5% × 1600 / 2000 = 0.04：右箭头把取景范围右移。
    expect(framing?.centerX).toBeCloseTo(0.54, 6);
  });

  it("加减号缩放，且钳在 1–3×", async () => {
    const wrapper = mountView();
    await frame(wrapper).trigger("keydown", { key: "+" });
    expect((wrapper.emitted("update:framing")?.at(-1)?.[0] as { zoom: number }).zoom).toBeCloseTo(1.1, 6);

    const slider = wrapper.get('[aria-label="缩放"]');
    await slider.setValue(String(SHAPE_FRAMING_ZOOM_MAX + 2));
    expect((wrapper.emitted("update:framing")?.at(-1)?.[0] as { zoom: number }).zoom).toBe(3);
    expect(wrapper.get('[aria-label="放大"]').attributes("disabled")).toBeDefined();

    await frame(wrapper).trigger("keydown", { key: "-" });
    expect((wrapper.emitted("update:framing")?.at(-1)?.[0] as { zoom: number }).zoom).toBeCloseTo(2.9, 6);
  });

  it("重置回到居中 1×（不是回到上一次的取景）", async () => {
    const wrapper = mountView();
    await frame(wrapper).trigger("keydown", { key: "ArrowRight" });
    await wrapper.get('[aria-label="重置取景"]').trigger("click");

    const framing = await confirmByButton(wrapper);
    expect(framing).toEqual(DEFAULT_SHAPE_FRAMING);
  });

  it("Esc 取消：不上抛任何取景状态", async () => {
    const wrapper = mountView();
    await frame(wrapper).trigger("keydown", { key: "Escape" });

    expect(wrapper.emitted("cancel")).toHaveLength(1);
    expect(wrapper.emitted("confirm")).toBeUndefined();
  });
});

describe("ImageCropView · 确认与取消是两个出口", () => {
  it("取消按钮只抛 cancel，不抛 confirm（取消＝中止本次上传）", async () => {
    const wrapper = mountView();
    await wrapper.get('[aria-label="取消裁剪"]').trigger("click");

    expect(wrapper.emitted("cancel")).toHaveLength(1);
    expect(wrapper.emitted("confirm")).toBeUndefined();
  });

  it("确认抛的是钳位后的取景状态（缩放/中心都在合法范围内）", async () => {
    const wrapper = mountView();
    const slider = wrapper.get('[aria-label="缩放"]');
    await slider.setValue("0.4");

    const framing = await confirmByButton(wrapper);
    expect(framing?.zoom).toBe(1);
    expect(framing?.centerX).toBe(0.5);
    expect(framing?.centerY).toBe(0.5);
  });

  it("没有旋转 / 镜像 / 滤镜入口：只有拖动、缩放、重置、确认、取消", () => {
    const wrapper = mountView();
    const labels = wrapper.findAll("button").map((button) => button.attributes("aria-label"));

    expect(labels).toEqual(["缩小", "放大", "重置取景", "取消裁剪", "确认裁剪"]);
    expect(wrapper.html()).not.toMatch(/rotate|scaleX|mirror|filter:/u);
  });
});

/**
 * 取景框几何（票 242）：以前框是 `width: 100%` + aspect-ratio，高度按宽度派生 →
 * 头像 1:1 时高度≈弹层内容宽，弹层被撑满。改成**由高度决定宽**：高度预算来自
 * 视口（`min(46dvh, 360px)`），宽度 = 预算 × 比例，且不超过台面宽。
 *
 * happy-dom 不跑布局，这里能观察的是组件交给 DOM 的样式声明，以及"底部操作条
 * 不在滚动区里"的结构契约；滚动/不滚由组件样式里的同一条声明保证。
 */
describe("ImageCropView · 取景框由高度决定宽（票 242）", () => {
  const viewSource = readFileSync(
    join(process.cwd(), "app/components/studio/ImageCropView.vue"),
    "utf8",
  );

  function frameStyle(wrapper: ReturnType<typeof mountView>): string {
    return frame(wrapper).attributes("style") ?? "";
  }

  /** 框交出的比例乘数（`--crop-frame-ratio: 1.777778` 里的 1.777778）。 */
  function widthMultiplier(style: string): number {
    const raw = /--crop-frame-ratio:\s*([0-9.]+)/u.exec(style)?.[1];
    expect(raw, "框必须把目标比例交给样式里的宽度公式").toBeDefined();
    return Number(raw);
  }

  /** 组件样式里的一条规则块（happy-dom 不加载 scoped CSS，只能看源文件声明）。 */
  function cssBlock(selector: string): string {
    const start = viewSource.indexOf(`${selector} {`);
    expect(start, `${selector} 规则必须在组件样式里`).toBeGreaterThan(-1);
    return viewSource.slice(start, viewSource.indexOf("}", start));
  }

  it("高度预算是视口高度的 46%、桌面封顶 360px；宽度按比例反推且不超过台面", () => {
    const frameCss = cssBlock(".image-crop__frame");
    const style = frameStyle(mountView());

    // 尺寸方向写在样式里：宽度 = 高度预算 × 比例，且不超过台面宽。
    expect(frameCss).toContain("min(46dvh, 360px)");
    expect(frameCss).toContain("min(100%");
    expect(frameCss).toContain("var(--crop-frame-ratio");
    // 封面 16:9：宽上限 = 高度预算 × 16/9（独立来源：规格里的 16:9）。
    expect(widthMultiplier(style)).toBeCloseTo(16 / 9, 4);
    // 框的宽度不再由"整条撑满"决定（max-width 除外）。
    expect(frameCss).not.toMatch(/(?<!max-)width:\s*100%/u);
  });

  it("头像 1:1 的宽上限就是 360px 高度预算，不再等于弹层内容宽", () => {
    const style = frameStyle(mountView({ target: { width: 256, height: 256 }, ratioText: "1:1" }));

    // min(100%, 360px × 1)：台面再宽，1:1 框也只有 360×360。
    expect(widthMultiplier(style)).toBeCloseTo(1, 6);
    expect(style).toMatch(/--crop-frame-ratio:\s*1(;|$)/u);
  });

  it("内容区自己滚、底部动作条不滚：操作条在滚动区之外", () => {
    const wrapper = mountView();
    const body = wrapper.get('[data-role="crop-body"]');

    expect(body.find('[data-role="crop-frame"]').exists()).toBe(true);
    expect(body.find('[aria-label="缩放"]').exists()).toBe(true);
    expect(body.find('[data-role="crop-actions"]').exists()).toBe(false);

    const actions = wrapper.get('[data-role="crop-actions"]');
    expect(actions.findAll("button").map((button) => button.attributes("aria-label"))).toEqual([
      "重置取景",
      "取消裁剪",
      "确认裁剪",
    ]);

    expect(cssBlock(".image-crop__body")).toContain("overflow-y: auto");
    expect(cssBlock(".image-crop__actions")).toContain("flex: none");
    // 框不再是"整条撑满 + 比例派生高度"。
    expect(cssBlock(".image-crop__frame")).not.toMatch(/(?<!max-)width:\s*100%/u);
  });
});
