import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import StudioCoverUploadPanel from "~/components/studio/StudioCoverUploadPanel.vue";
import { COVER_SHAPE_SIZE_TEXT } from "~/utils/imageShaping";

/**
 * 封面上传面板卡（与后台轮播上传同一套语言；票 200 起上传前定形，#204 取景退场）：
 *   - 定形口径（16:9 / 1600×900）与支持格式常驻可见；
 *   - 悬停/忙碌是同一张卡的两个状态（忙碌时不能再选文件）；
 *   - 尺寸与「已定形」徽标只在选过图后出现，没有任何取景入口；
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
      onFileChange: vi.fn(),
      onClear: vi.fn(),
      ...props,
    },
  });
}

describe("StudioCoverUploadPanel · 空态与提示", () => {
  it("是一张面板卡：标题、定形口径与支持格式都在", () => {
    const wrapper = mountPanel();

    expect(wrapper.get(".cover-panel").exists()).toBe(true);
    expect(wrapper.get(".cover-panel__title").text()).toContain("文章封面");
    expect(wrapper.get(".cover-panel__hint").text()).toContain("上传前自动裁成 16:9");
    expect(wrapper.get(".cover-panel__format").text()).toContain("1600 × 900");
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

  it("没有任何取景入口（#204 取景窗退场）：选过图也不出现「手动/调整取景」按钮", () => {
    const wrapper = mountPanel({ size: { width: 1600, height: 900 } });

    expect(wrapper.find('[aria-label="手动取景"]').exists()).toBe(false);
    expect(wrapper.find('[aria-label="调整取景"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain("取景");
  });
});

describe("StudioCoverUploadPanel · 定形后的尺寸与徽标", () => {
  it("定形产物给出尺寸读数与 success 徽标", () => {
    const wrapper = mountPanel({ size: { width: 1600, height: 900 } });

    expect(wrapper.get(".cover-panel__size").text()).toBe("1600 × 900");
    expect(wrapper.get(".cover-panel__badge").text()).toBe(`已定形 ${COVER_SHAPE_SIZE_TEXT}`);
  });

  it("回填的旧封面没有定形产物尺寸：不显示徽标", () => {
    const wrapper = mountPanel({ previewUrl: "https://cdn.example.com/old.png" });

    expect(wrapper.find(".cover-panel__meta").exists()).toBe(false);
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

  it("选文件、移除两个入口把事件原样交给父级", async () => {
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
