import { describe, expect, it } from "vitest";

import {
  SEARCH_ACTION_LABELS,
  resolveSearchEmptyState,
  type SearchEmptyContext,
} from "~/utils/searchState";

const baseContext: SearchEmptyContext = {
  mode: "normal",
  key: "",
  tagId: "",
  pending: false,
  failed: false,
};

describe("searchState：搜索空态 / 错误态与下一步指引", () => {
  it("加载中：普通搜索与智能搜索给出不同的等待文案，且不显示操作按钮", () => {
    const normal = resolveSearchEmptyState({ ...baseContext, key: "nuxt", pending: true });
    const ai = resolveSearchEmptyState({ ...baseContext, mode: "ai", key: "怎么部署", pending: true });

    expect(normal.kind).toBe("loading");
    expect(normal.title).toContain("正在搜索");
    expect(normal.actions).toEqual([]);
    expect(ai.kind).toBe("loading");
    expect(ai.title).toContain("正在理解");
  });

  it("请求失败：错误态给出重试 / 清空 / 换方式 / 回首页这一组下一步", () => {
    const normal = resolveSearchEmptyState({ ...baseContext, key: "nuxt", failed: true });
    const ai = resolveSearchEmptyState({ ...baseContext, mode: "ai", key: "怎么部署", failed: true });

    expect(normal.kind).toBe("error");
    expect(normal.title).toBe("搜索暂时不可用");
    expect(normal.actions).toEqual(["retry", "clear-filters", "switch-mode", "open-home"]);
    expect(normal.hints.length).toBeGreaterThan(0);
    expect(ai.kind).toBe("error");
    expect(ai.description).toContain("智能搜索");
  });

  it("还没输入（普通搜索）：只留标题与下一步指引，不再复述顶部说明（#126）", () => {
    const state = resolveSearchEmptyState(baseContext);

    expect(state.kind).toBe("idle");
    expect(state.title).toBe("输入关键词开始搜索");
    // 与顶部说明重复的那句描述已删；引导由 hints + actions 承担。
    expect(state.description).toBe("");
    expect(state.hints.length).toBeGreaterThan(0);
    expect(state.actions).toEqual(["switch-mode", "open-home"]);
  });

  it("智能搜索但没输入：引导先描述想找什么", () => {
    const state = resolveSearchEmptyState({ ...baseContext, mode: "ai" });

    expect(state.kind).toBe("ai-needs-query");
    expect(state.title).toBe("先描述你想找什么");
    expect(state.hints.length).toBeGreaterThan(0);
    expect(state.actions).toEqual(["switch-mode", "open-home"]);
  });

  it("有筛选但无结果：判为 no-match，并给出清空条件 / 换方式的指引", () => {
    const byTag = resolveSearchEmptyState({ ...baseContext, tagId: "t1" });
    const byKey = resolveSearchEmptyState({ ...baseContext, key: "nuxt" });
    const ai = resolveSearchEmptyState({ ...baseContext, mode: "ai", key: "怎么部署" });

    expect(byTag.kind).toBe("no-match");
    expect(byKey.kind).toBe("no-match");
    expect(byTag.title).toBe("没有匹配的文章");
    expect(byTag.actions).toContain("clear-filters");
    expect(byKey.actions).not.toContain("retry");
    expect(ai.kind).toBe("no-match");
    expect(ai.description).toContain("描述");
  });

  it("操作按钮标签齐全（页面直接用这组中文文案）", () => {
    expect(SEARCH_ACTION_LABELS.retry).toBe("重试本页");
    expect(SEARCH_ACTION_LABELS["clear-filters"]).toBe("清空条件");
    expect(SEARCH_ACTION_LABELS["switch-mode"]).toBe("换个搜索方式");
    expect(SEARCH_ACTION_LABELS["open-home"]).toBe("回到首页");
  });
});
