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
