import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import CommentComposerSheet from "~/components/comment/CommentComposerSheet.vue";

/**
 * 评论输入面板（决定清单 §4 第 19–22 条）。
 *
 * 规格里有一半是"怎么关"，这些只能靠挂载后的点击/按键断言，源码 grep 证不了：
 *   1. 点面板外（透明捕获层）收起；
 *   2. Esc 收起；
 *   3. 点右上 × 收起；
 *   4. 面板内**只有一个**提交按钮（取消 / 清空已退役），且没有登录提示；
 *   5. 提交把内容原样抛给页面（页面负责调接口 + 关面板）。
 */

vi.mock("~/composables/useReaderLoginGate", () => ({
  useReaderLoginGate: () => async () => true,
}));

function mountSheet(props: Record<string, unknown> = {}) {
  return mount(CommentComposerSheet, { props: { show: true, ...props }, attachTo: document.body });
}

describe("CommentComposerSheet：三条关闭途径", () => {
  it("点面板外（透明捕获层）收起", async () => {
    const wrapper = mountSheet();

    await wrapper.get('[data-role="comment-sheet-scrim"]').trigger("click");

    expect(wrapper.emitted("update:show")?.[0]).toEqual([false]);
    wrapper.unmount();
  });

  it("按 Esc 收起", async () => {
    const wrapper = mountSheet();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("update:show")?.[0]).toEqual([false]);
    wrapper.unmount();
  });

  it("点右上 × 收起", async () => {
    const wrapper = mountSheet();

    await wrapper.get('[data-role="comment-sheet-close"]').trigger("click");

    expect(wrapper.emitted("update:show")?.[0]).toEqual([false]);
    wrapper.unmount();
  });

  it("收起时不渲染任何东西（面板与捕获层都不在）", () => {
    const wrapper = mount(CommentComposerSheet, { props: { show: false } });

    expect(wrapper.find('[data-role="comment-sheet"]').exists()).toBe(false);
    expect(wrapper.find('[data-role="comment-sheet-scrim"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe("CommentComposerSheet：面板内部", () => {
  it("只有一个提交按钮，没有取消 / 清空，也没有登录提示", () => {
    const wrapper = mountSheet({ title: "回复 张三", submitLabel: "发送回复" });

    expect(wrapper.findAll('[data-role="comment-submit"]')).toHaveLength(1);
    expect(wrapper.text()).toContain("发送回复");
    expect(wrapper.text()).not.toContain("取消");
    expect(wrapper.text()).not.toContain("登录后");
    wrapper.unmount();
  });

  it("面板带标题行（发表评论 / 回复 X）与抓手，语义在 dialog 上", () => {
    const wrapper = mountSheet({ title: "回复 张三" });
    const dialog = wrapper.get('[role="dialog"]');

    expect(dialog.attributes("aria-label")).toBe("回复 张三");
    expect(wrapper.find(".comment-sheet__grabber").exists()).toBe(true);
    wrapper.unmount();
  });

  it("提交把内容原样抛给页面（页面负责调接口与关面板）", async () => {
    const wrapper = mountSheet();

    await wrapper.get("textarea").setValue("  我的看法  ");
    await wrapper.get('[data-role="comment-submit"]').trigger("click");

    expect(wrapper.emitted("submit")?.[0]).toEqual(["我的看法"]);
    wrapper.unmount();
  });
});
