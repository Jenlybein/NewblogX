import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";

import ImageCropDialog from "~/components/studio/ImageCropDialog.vue";
import { COVER_SHAPE_RATIO_TEXT, COVER_SHAPE_TARGET } from "~/utils/imageShaping";

/**
 * 裁剪弹层外壳（票 242）：以前 `NModal` 只传 `preset="card"`，没有 width——
 * naive-ui 的 `NCard` 自带 `width: 100%`、`.n-modal` 没有宽度，于是弹层被最长
 * 中文提示行撑到接近窗口宽；再加上取景框按宽度派生高度，观感就是"铺满窗口"。
 *
 * 这里断言的是**外壳交给 naive-ui 什么尺寸配置**，以及三个出口：
 * 确认（带上取景状态）、取消、弹层自己发起的关闭（右上角 X / Esc）也算取消。
 * naive-ui 被替换成只记录 prop 的壳：naive-ui 自己的渲染不是这一层的职责。
 */
const rendered = vi.hoisted(() => [] as Array<Record<string, unknown>>);

vi.mock("naive-ui", () => {
  const NModal = defineComponent({
    name: "NModal",
    props: {
      show: { type: Boolean, default: false },
      title: { type: String, default: "" },
      bordered: { type: Boolean, default: true },
      maskClosable: { type: Boolean, default: true },
      style: { type: [String, Object], default: undefined },
    },
    emits: ["update:show"],
    setup(props, { slots, emit }) {
      return () => {
        if (!props.show) return null;
        rendered.push({ ...props });
        return h("div", { "data-role": "n-modal", "data-style": JSON.stringify(props.style ?? null) }, [
          slots.default?.(),
          // 代表右上角 X / Esc：弹层自己发起的关闭。
          h("button", { "data-role": "n-modal-close", onClick: () => emit("update:show", false) }, "×"),
        ]);
      };
    },
  });

  return { NModal };
});

function mountDialog(overrides: Record<string, unknown> = {}) {
  return mount(ImageCropDialog, {
    props: {
      show: true,
      src: "blob:original",
      imageWidth: 2000,
      imageHeight: 900,
      target: COVER_SHAPE_TARGET,
      ratioText: COVER_SHAPE_RATIO_TEXT,
      title: "裁剪封面",
      ...overrides,
    },
  });
}

type Dialog = ReturnType<typeof mountDialog>;

function modalStyle(wrapper: Dialog): Record<string, string> {
  const raw = wrapper.get('[data-role="n-modal"]').attributes("data-style");
  return JSON.parse(raw ?? "{}") as Record<string, string>;
}

function confirmByButton(wrapper: Dialog) {
  return wrapper.get('[aria-label="确认裁剪"]').trigger("click");
}

describe("ImageCropDialog · 弹层尺寸（票 242）", () => {
  it("宽度封顶 560px；92vw 在移动端恰好留出左右各 4vw 边距", () => {
    const style = modalStyle(mountDialog());

    expect(style.width).toBe("min(92vw, 560px)");
    // 92vw + 2 × 4vw = 100vw：窄屏不再贴边。
    expect(style.width).toContain("92vw");
  });

  it("高度不超过视口：max-height 是 100dvh 减去上下 24px 留白", () => {
    expect(modalStyle(mountDialog()).maxHeight).toBe("calc(100dvh - 48px)");
  });

  it("遮罩误触仍然关不掉（尺寸变化不放松这条约束）", () => {
    mountDialog();
    const props = rendered.at(-1) ?? {};

    expect(props.maskClosable).toBe(false);
    expect(props.bordered).toBe(false);
  });
});

describe("ImageCropDialog · 三个出口（确认 / 取消 / 弹层自关）", () => {
  it("点确认：上抛取景状态并关窗", async () => {
    const wrapper = mountDialog();
    await confirmByButton(wrapper);

    const framing = wrapper.emitted("confirm")?.at(-1)?.[0];
    expect(framing).toEqual({ zoom: 1, centerX: 0.5, centerY: 0.5 });
    expect(wrapper.emitted("update:show")?.at(-1)?.[0]).toBe(false);
  });

  it("点取消：只抛 cancel，不上传任何取景状态", async () => {
    const wrapper = mountDialog();
    await wrapper.get('[aria-label="取消裁剪"]').trigger("click");

    expect(wrapper.emitted("cancel")).toHaveLength(1);
    expect(wrapper.emitted("confirm")).toBeUndefined();
  });

  it("弹层自己关（X / Esc / 遮罩）也算取消；确认之后的关窗不再重复结算", async () => {
    const first = mountDialog();
    await first.get('[data-role="n-modal-close"]').trigger("click");
    expect(first.emitted("cancel")).toHaveLength(1);
    expect(first.emitted("confirm")).toBeUndefined();

    const second = mountDialog();
    await confirmByButton(second);
    await second.get('[data-role="n-modal-close"]').trigger("click");
    // 确认后弹层的 update:show(false) 与确认是同一次开关的收尾：不能再抛 cancel。
    expect(second.emitted("cancel")).toBeUndefined();
    expect(second.emitted("confirm")).toHaveLength(1);
  });
});
