import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import StudioCoverFramingView from "~/components/studio/StudioCoverFramingView.vue";

/**
 * 文章封面取景窗的三条交互（验收口径）：
 *   1. 拖动产出取景参数；
 *   2. 缩放产出取景参数；
 *   3. 取消不上抛任何取景参数（＝本次上传中止）。
 * 外加键盘平移/缩放。
 *
 * 断言只看对外可观察的东西：组件抛出的参数、图片上最终生效的 CSS 声明。
 * 选择器用语义 class 与 aria-label，不断言内部 ref 名或私有结构。
 */

const FRAME = { width: 800, height: 450 };

function mountView(props: Record<string, unknown> = {}) {
  return mount(StudioCoverFramingView, {
    props: {
      src: "/demo.jpg",
      imageWidth: 3200,
      imageHeight: 900,
      ...props,
    },
  });
}

function pointer(type: string, clientX: number, clientY: number) {
  return new MouseEvent(type, { bubbles: true, clientX, clientY });
}

describe("StudioCoverFramingView", () => {
  it("拖动产出取景参数，并让图片按同一焦点铺满取景框", async () => {
    const wrapper = mountView();

    const frame = wrapper.get(".cover-framing__frame");
    frame.element.dispatchEvent(pointer("pointerdown", 400, 200));
    window.dispatchEvent(pointer("pointermove", 480, 200));
    window.dispatchEvent(pointer("pointerup", 480, 200));
    await wrapper.vm.$nextTick();

    const updates = wrapper.emitted("update:framing") as unknown as [Record<string, number>][];
    // 3200×900 的宽幅图：往右拖 80px，焦点 x 从 0.5 减 0.1。
    expect(updates.at(-1)?.[0]).toEqual({ focus_x: 0.4, focus_y: 0.5, zoom: 1 });

    expect(wrapper.get(".cover-framing__image").attributes("style")).toContain("object-position: 40% 50%");

    // 一路拖到底：焦点贴边 0/1，不会变成负数，也不会让 object-position 出界（露白）。
    frame.element.dispatchEvent(pointer("pointerdown", 400, 200));
    window.dispatchEvent(pointer("pointermove", 99999, 200));
    window.dispatchEvent(pointer("pointerup", 99999, 200));
    await wrapper.vm.$nextTick();

    expect(wrapper.get(".cover-framing__image").attributes("style")).toContain("object-position: 0% 50%");
  });

  it("缩放滑杆与加减号都按 0.05 步进，且钳在 1–2", async () => {
    const wrapper = mountView();

    await wrapper.get(".cover-framing__zoom").setValue("1.5");
    const updates = wrapper.emitted("update:framing") as unknown as [Record<string, number>][];
    expect(updates.at(-1)?.[0]).toEqual({ focus_x: 0.5, focus_y: 0.5, zoom: 1.5 });

    await wrapper.get('[aria-label="放大"]').trigger("click");
    expect((wrapper.emitted("update:framing") as unknown as [Record<string, number>][])?.at(-1)?.[0].zoom).toBe(1.55);

    // 运到上界后按钮禁用，不会越过 2。
    for (let step = 0; step < 12; step += 1) {
      await wrapper.get('[aria-label="放大"]').trigger("click");
    }
    expect(wrapper.get('[aria-label="放大"]').attributes("disabled")).toBeDefined();
    expect((wrapper.emitted("update:framing") as unknown as [Record<string, number>][]).at(-1)?.[0].zoom).toBe(2);
  });

  it("取消不上抛任何取景参数（也不抛确认）", async () => {
    const wrapper = mountView();

    await wrapper.get('[aria-label="取消取景"]').trigger("click");

    expect(wrapper.emitted("cancel")).toHaveLength(1);
    expect(wrapper.emitted("confirm")).toBeUndefined();
  });

  it("确认取景上抛钳制后的三个参数", async () => {
    const wrapper = mountView();

    await wrapper.get('[aria-label="重置取景"]').trigger("click");
    await wrapper.get('[aria-label="确认取景"]').trigger("click");

    expect(wrapper.emitted("confirm")).toEqual([[{ focus_x: 0.5, focus_y: 0.5, zoom: 1 }]]);
  });

  it("键盘可操作：方向键移动取景范围、加减号缩放", async () => {
    const wrapper = mountView({ framing: { focus_x: 0.5, focus_y: 0.5, zoom: 1 } });

    const frame = wrapper.get(".cover-framing__frame");
    await frame.trigger("keydown", { key: "ArrowRight" });
    await frame.trigger("keydown", { key: "ArrowUp" });

    for (let step = 0; step < 10; step += 1) {
      await frame.trigger("keydown", { key: "+" });
    }
    await frame.trigger("keydown", { key: "-" });

    const updates = wrapper.emitted("update:framing") as unknown as [Record<string, number>][];
    const last = updates.at(-1)?.[0];
    expect(last?.focus_x).toBeCloseTo(0.52, 4);
    expect(last?.focus_y).toBeCloseTo(0.48, 4);
    expect(last?.zoom).toBe(1.45);
  });

  it("传入已有取景参数时按它开场（重新打开不改动旧值）", async () => {
    const wrapper = mountView({ framing: { focus_x: 0.25, focus_y: 0.75, zoom: 1.5 } });

    expect(wrapper.get(".cover-framing__image").attributes("style")).toContain("object-position: 25% 75%");
    await wrapper.get('[aria-label="确认取景"]').trigger("click");
    expect(wrapper.emitted("confirm")).toEqual([[{ focus_x: 0.25, focus_y: 0.75, zoom: 1.5 }]]);
  });
});
