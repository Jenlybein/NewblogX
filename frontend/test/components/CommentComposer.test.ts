import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h, ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import CommentComposer from "~/components/comment/CommentComposer.vue";

/**
 * 评论输入本的对外协议（2026-11-04 决定清单 §4 第 18/24/25 条）。
 *
 * 三件事：
 *  1. 它现在长在底部滑出面板里，只负责**输入 + 提交**；聚焦由面板调用 `focus()`；
 *  2. 登录提示与写死的装饰标签行都已退役（未登录点入口就弹登录框，语义自明）；
 *  3. 未登录提交只弹登录框、不发请求（否则请求层会把登录态打成失效）。
 */

const gateState = vi.hoisted(() => ({ allow: true, openCount: 0 }));

vi.mock("~/composables/useReaderLoginGate", () => ({
  useReaderLoginGate: () => async () => {
    gateState.openCount += 1;
    return gateState.allow;
  },
}));

vi.mock("naive-ui", () => {
  // 只做 NInput 的最小替身：真实 textarea + 暴露 focus()/textareaElRef，
  // 这样"聚焦到输入框"是被真实 DOM 焦点验证的，不是断言一个假方法被调用。
  const NInput = defineComponent({
    name: "NInput",
    props: {
      value: { type: String, default: "" },
    },
    emits: ["update:value"],
    setup(props, { emit, expose }) {
      const textareaElRef = ref<HTMLTextAreaElement | null>(null);
      expose({
        textareaElRef,
        focus: () => textareaElRef.value?.focus(),
      });
      return () =>
        h("textarea", {
          ref: textareaElRef,
          value: props.value,
          "aria-label": "评论输入框",
          onInput: (event: Event) => emit("update:value", (event.target as HTMLTextAreaElement).value),
        });
    },
  });

  return { NInput };
});

function mountComposer(props: Record<string, unknown> = {}) {
  return mount(CommentComposer, { props });
}

describe("CommentComposer：文案精简", () => {
  it("没有任何登录提示（提示随内嵌输入框一起退役）", () => {
    for (const loggedIn of [true, false]) {
      const wrapper = mountComposer({ loggedIn });

      expect(wrapper.text()).not.toContain("已登录");
      expect(wrapper.text()).not.toContain("登录后");
      expect(wrapper.find(".comment-composer__status").exists()).toBe(false);
    }
  });

  it("写死的装饰标签行退役（假标签会让人以为能选）", () => {
    const wrapper = mountComposer();

    expect(wrapper.find(".comment-composer__tags").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("接口设计");
  });
});

describe("CommentComposer：聚焦", () => {
  it("focus() 只聚焦 textarea，不滚动页面（面板负责位置）", () => {
    const scrollIntoView = vi.fn();
    const originalScroll = HTMLElement.prototype.scrollIntoView;
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    try {
      const wrapper = mount(CommentComposer, { attachTo: document.body });
      wrapper.vm.focus();

      expect(scrollIntoView).not.toHaveBeenCalled();
      expect(document.activeElement).toBe(wrapper.get("textarea").element);
      wrapper.unmount();
    } finally {
      HTMLElement.prototype.scrollIntoView = originalScroll;
    }
  });
});

describe("CommentComposer：提交与登录前置", () => {
  beforeEach(() => {
    gateState.allow = true;
    gateState.openCount = 0;
  });

  it("已登录：提交去掉首尾空白的内容并清空输入框", async () => {
    const wrapper = mountComposer({ loggedIn: true });
    await wrapper.get("textarea").setValue("  写点什么  ");
    await wrapper.get("button.article-action--primary").trigger("click");
    await flushPromises();

    expect(wrapper.emitted("submit")?.[0]).toEqual(["写点什么"]);
    expect((wrapper.get("textarea").element as HTMLTextAreaElement).value).toBe("");
  });

  it("未登录：只弹登录框，不发提交（页面不产生任何请求）", async () => {
    gateState.allow = false;
    const wrapper = mountComposer({ loggedIn: false });
    await wrapper.get("textarea").setValue("游客的评论");
    await wrapper.get("button.article-action--primary").trigger("click");
    await flushPromises();

    expect(gateState.openCount).toBe(1);
    expect(wrapper.emitted("submit")).toBeUndefined();
  });

  it("已登录但内容为空：不提交", async () => {
    const wrapper = mountComposer({ loggedIn: true });
    await wrapper.get("button.article-action--primary").trigger("click");
    await flushPromises();

    expect(wrapper.emitted("submit")).toBeUndefined();
  });
});
