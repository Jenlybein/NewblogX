import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h, ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import CommentComposer from "~/components/comment/CommentComposer.vue";

/**
 * 评论输入框的对外协议（票 238 范围 3/6/7）。
 *
 * 三件事：
 *  1. 点「回复」后页面要能把握手权交回这里（`focusInput()`：滚动到框 + `focus()`），
 *     所以 textarea 的 ref / 聚焦方法必须暴露出去；
 *  2. 「已登录，可直接评论」这句解释性文案删掉，未登录仍提示「登录后可参与讨论」；
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

describe("CommentComposer：登录提示文案", () => {
  it("已登录时不出现「已登录，可直接评论」这类解释", () => {
    const wrapper = mountComposer({ loggedIn: true });

    expect(wrapper.text()).not.toContain("已登录");
    expect(wrapper.find(".comment-composer__status").exists()).toBe(false);
  });

  it("未登录仍提示「登录后可参与讨论」", () => {
    const wrapper = mountComposer({ loggedIn: false });

    expect(wrapper.get(".comment-composer__status").text()).toBe("登录后可参与讨论");
  });
});

describe("CommentComposer：聚焦与滚动", () => {
  it("focusInput() 把评论框滚进视口并聚焦 textarea（点「回复」后页面直接调它）", async () => {
    const scrollIntoView = vi.fn();
    const originalScroll = HTMLElement.prototype.scrollIntoView;
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    try {
      // 挂到真实 document 上：`focus()` 只有元素在文档里才会真的改变 activeElement。
      const wrapper = mount(CommentComposer, { props: { loggedIn: true }, attachTo: document.body });
      wrapper.vm.focusInput();

      expect(scrollIntoView).toHaveBeenCalledTimes(1);
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
