import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import type { ReaderRenderMode } from "~/utils/readerRenderMode";
import type { ArticleOutlineContext } from "~/utils/outlineContext";

/**
 * 阅读页左栏取数的时序（接缝四/五）：树只在客户端取数，服务端不注入；
 * 站内跳转先出骨架（pending），拿到树即 ready，无归属/失败一律降级为不显示左栏。
 *
 * 这里用最小挂载组件观察用例，不依赖整张阅读页（它是 700 行的 SSR 路由页）。
 */

const ARTICLE_ID = "101";

function makeContext(overrides: Partial<ArticleOutlineContext> = {}): ArticleOutlineContext {
  return {
    has_tree: true,
    owner_kind: 2,
    owner_id: "375",
    nodes: [
      { id: "n1", kind: 1, title: "第一章", children: [{ id: "n2", kind: 2, article_id: ARTICLE_ID }] },
    ],
    current: { id: "n2", article_id: ARTICLE_ID, article_title: "篇名" },
    previous: null,
    next: null,
    ...overrides,
  };
}

function mountContext(options: {
  mode?: ReaderRenderMode;
  from?: string;
  articleId?: string;
  fetch: (articleId: string, from?: string) => Promise<ArticleOutlineContext>;
}) {
  const from = ref(options.from ?? "");
  const articleId = ref(options.articleId ?? ARTICLE_ID);
  const Harness = defineComponent({
    setup() {
      return {
        ...useArticleOutlineContext({
          articleId,
          from,
          mode: options.mode ?? "navigation",
          fetch: options.fetch,
        }),
        from,
      };
    },
    template: `<div data-article-outline-state="true" />`,
  });

  return mount(Harness);
}

describe("useArticleOutlineContext：阅读页左栏取数时序", () => {
  it("站内跳转立刻进入 pending（骨架先上屏），拿到树后 ready", async () => {
    const fetchOutline = vi.fn(async () => makeContext());
    const wrapper = mountContext({ fetch: fetchOutline });

    expect(wrapper.vm.status).toBe("pending");
    expect(wrapper.vm.active).toBe(true);
    expect(wrapper.vm.pending).toBe(true);

    await flushPromises();

    expect(fetchOutline).toHaveBeenCalledTimes(1);
    expect(fetchOutline).toHaveBeenCalledWith(ARTICLE_ID, "");
    expect(wrapper.vm.status).toBe("ready");
    expect(wrapper.vm.hasTree).toBe(true);
    expect(wrapper.vm.context?.owner_id).toBe("375");
  });

  it("服务端渲染模式完全不取数（SSR 边界与匿名缓存不被左栏污染，ADR-0019）", async () => {
    const fetchOutline = vi.fn(async () => makeContext());
    const wrapper = mountContext({ mode: "server", fetch: fetchOutline });

    await flushPromises();

    expect(fetchOutline).not.toHaveBeenCalled();
    expect(wrapper.vm.status).toBe("idle");
    expect(wrapper.vm.active).toBe(false);
  });

  it("无归属树：结果空、左栏不活跃（页面右栏标题大纲不受影响）", async () => {
    const fetchOutline = vi.fn(async () => makeContext({ has_tree: false, owner_id: "", nodes: [], current: null }));
    const wrapper = mountContext({ fetch: fetchOutline });

    await flushPromises();

    expect(wrapper.vm.status).toBe("empty");
    expect(wrapper.vm.hasTree).toBe(false);
    expect(wrapper.vm.active).toBe(false);
    expect(wrapper.vm.context).toBeNull();
  });

  it("取数失败降级为 empty：左栏不出现，不把阅读页带崩", async () => {
    const fetchOutline = vi.fn(async () => {
      throw new Error("network down");
    });
    const wrapper = mountContext({ fetch: fetchOutline });

    await flushPromises();

    expect(wrapper.vm.status).toBe("empty");
    expect(wrapper.vm.active).toBe(false);
  });

  it("from 原样透传；来源变化重新取数", async () => {
    const fetchOutline = vi.fn(async () => makeContext({ owner_kind: 1, owner_id: "9" }));
    const wrapper = mountContext({ from: "project:375", fetch: fetchOutline });

    await flushPromises();
    expect(fetchOutline).toHaveBeenCalledWith(ARTICLE_ID, "project:375");

    wrapper.vm.from = "collection:9";
    await flushPromises();
    expect(fetchOutline).toHaveBeenLastCalledWith(ARTICLE_ID, "collection:9");
  });

  it("文章 ID 为空时不发请求（详情页错误态不额外取数）", async () => {
    const fetchOutline = vi.fn(async () => makeContext());
    const wrapper = mountContext({ articleId: "", fetch: fetchOutline });

    await flushPromises();

    expect(fetchOutline).not.toHaveBeenCalled();
    expect(wrapper.vm.active).toBe(false);
  });
});

import { useArticleOutlineContext } from "~/composables/useArticleOutlineContext";
