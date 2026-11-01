import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import StudioCoverUploadPanel from "~/components/studio/StudioCoverUploadPanel.vue";

/**
 * 封面上传面板卡（与后台轮播上传同一套语言）：
 *   - 推荐比例 + 格式提示常驻可见；
 *   - 悬停/忙碌是同一张卡的两个状态（忙碌时不能再选文件）；
 *   - 尺寸与合规徽标只在选过图后出现；
 *   - 上传失败的文案留在面板里（role=alert），不是一闪而过的 toast。
 */

function mountPanel(props: Record<string, unknown> = {}) {
  return mount(StudioCoverUploadPanel, {
    props: {
      busy: false,
      previewUrl: "",
      stage: "",
      error: "",
      size: null,
      route: "",
      framed: false,
      onFileChange: vi.fn(),
      onClear: vi.fn(),
      onFrame: vi.fn(),
      ...props,
    },
  });
}

describe("StudioCoverUploadPanel · 空态与提示", () => {
  it("是一张面板卡：标题、推荐比例与支持格式都在", () => {
    const wrapper = mountPanel();

    expect(wrapper.get(".cover-panel").exists()).toBe(true);
    expect(wrapper.get(".cover-panel__title").text()).toContain("文章封面");
    expect(wrapper.get(".cover-panel__hint").text()).toContain("推荐比例 16:9");
    expect(wrapper.get(".cover-panel__format").text()).toMatch(/JPG/i);
    expect(wrapper.get(".cover-panel__format").text()).toMatch(/WebP/i);
  });

  it("空态显示加号与「上传封面」，不渲染尺寸/徽标/错误", () => {
    const wrapper = mountPanel();

    expect(wrapper.get(".cover-panel__plus").text()).toBe("+");
    expect(wrapper.text()).toContain("上传封面");
    expect(wrapper.find(".cover-panel__meta").exists()).toBe(false);
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
  });
});

describe("StudioCoverUploadPanel · 选图后的尺寸与合规徽标", () => {
  it("合规图给出尺寸读数与 success 徽标", () => {
    const wrapper = mountPanel({ size: { width: 1920, height: 1080 }, route: "direct" });

    expect(wrapper.get(".cover-panel__size").text()).toBe("1920 × 1080");
    expect(wrapper.get(".cover-panel__badge").text()).toBe("16:9 合规");
  });

  it("不合规图提示先取景，且出现「手动取景」入口", () => {
    const onFrame = vi.fn();
    const wrapper = mountPanel({ size: { width: 1200, height: 800 }, route: "frame", onFrame });

    expect(wrapper.get(".cover-panel__badge").text()).toBe("比例不合规，需取景");
    const frameButton = wrapper.get('[aria-label="手动取景"]');
    expect(frameButton.exists()).toBe(true);

    frameButton.trigger("click");
    expect(onFrame).toHaveBeenCalled();
  });

  it("取景确认后徽标改成「已取景」，入口变成「调整取景」", () => {
    const wrapper = mountPanel({ size: { width: 1200, height: 800 }, route: "frame", framed: true });

    expect(wrapper.get(".cover-panel__badge").text()).toBe("已取景");
    expect(wrapper.get('[aria-label="调整取景"]').exists()).toBe(true);
    expect(wrapper.find('[aria-label="手动取景"]').exists()).toBe(false);
  });
});

describe("StudioCoverUploadPanel · 忙碌态与错误态", () => {
  it("忙碌时卡片进入 busy 态，再点选择不会再弹文件框", () => {
    const wrapper = mountPanel({ busy: true, stage: "正在上传到对象存储…" });
    const input = wrapper.get(".cover-panel__input").element as HTMLInputElement;
    const click = vi.spyOn(input, "click").mockImplementation(() => {});

    const picker = wrapper.get(".cover-panel__picker");
    expect(picker.classes()).toContain("cover-panel__picker--busy");
    expect(picker.attributes("aria-busy")).toBe("true");
    expect(wrapper.get('[aria-label="选择图片"]').attributes("disabled")).toBeDefined();

    picker.trigger("click");
    expect(click).not.toHaveBeenCalled();
    expect(wrapper.get(".cover-panel__stage").text()).toContain("正在上传");
  });

  it("选文件、移除、取景三个入口把事件原样交给父级", async () => {
    const onFileChange = vi.fn();
    const onClear = vi.fn();
    const wrapper = mountPanel({ previewUrl: "https://cdn.example.com/cover.png", onFileChange, onClear });

    await wrapper.get(".cover-panel__input").trigger("change");
    expect(onFileChange).toHaveBeenCalledTimes(1);

    expect(wrapper.get(".cover-panel__image").attributes("src")).toBe("https://cdn.example.com/cover.png");
    await wrapper.get(".cover-panel__remove").trigger("click");
    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it("上传失败在面板里留下 role=alert 的错误文案", () => {
    const wrapper = mountPanel({ error: "网络断开，封面上传失败" });

    expect(wrapper.get('[role="alert"]').text()).toBe("网络断开，封面上传失败");
  });
});
