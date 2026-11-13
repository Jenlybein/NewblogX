import { describe, expect, it } from "vitest";

import { homeSearchLocation, pinnedArticleView, resolveHomeSectionState } from "~/utils/home";

// 这组降级态只属于首页「最新文章」区块：热门置顶是运营位，
// 无数据 / 加载中 / 失败都整块不渲染（ADR-0015 决策 5），没有可测的降级态。
//
// 右栏不再有纯函数级的"分区开关"可测：它由 `sidebarCardViews` 的卡片数据驱动，
// 可见性判定已随旧配置一起下线（见 `test/utils/sidebarCards.test.ts` 与
// `test/components/HomeSidebar.test.ts`）。
describe("最新文章区块的降级态", () => {
  it("加载中：徽标显示加载中，不显示错误/空态文案", () => {
    const state = resolveHomeSectionState({ pending: true, failed: false, itemCount: 0 });
    expect(state.kind).toBe("loading");
    expect(state.badge).toBe("加载中");
    expect(state.title).toContain("正在读取");
  });

  it("请求失败且无数据：错误态 + 可执行的重试指引", () => {
    const state = resolveHomeSectionState({ pending: false, failed: true, itemCount: 0 });
    expect(state.kind).toBe("error");
    expect(state.badge).toBe("暂不可用");
    expect(state.hints.length).toBeGreaterThan(0);
  });

  it("空数据：空态给出下一步（去搜索）", () => {
    const state = resolveHomeSectionState({ pending: false, failed: false, itemCount: 0 });
    expect(state.kind).toBe("empty");
    expect(state.badge).toBe("空");
    expect(state.description).toContain("最新");
  });

  it("有数据：ready；失败但有缓存条目时 ready 且徽标标注缓存", () => {
    expect(resolveHomeSectionState({ pending: false, failed: false, itemCount: 3 })).toMatchObject({
      kind: "ready",
      badge: "3 篇",
    });
    expect(resolveHomeSectionState({ pending: false, failed: true, itemCount: 2 })).toMatchObject({
      kind: "ready",
      badge: "2 篇（缓存）",
    });
  });
});

describe("搜索落点 / 置顶卡片视图", () => {
  it("首页搜索落点复用页头契约：/search 且空关键词不带 ?key=", () => {
    expect(homeSearchLocation()).toEqual({ path: "/search", query: {} });
  });

  it("置顶卡片归一化：链接、兜底标题/摘要、缺字段时安全", () => {
    expect(
      pinnedArticleView({
        id: "a1",
        title: "置顶文章",
        abstract: "",
        category_title: "工程",
        author_nickname: "作者甲",
        view_count: 120,
      }),
    ).toMatchObject({
      id: "a1",
      href: "/article/a1",
      title: "置顶文章",
      abstract: "这篇文章暂时没有摘要。",
      categoryLabel: "工程",
      authorName: "作者甲",
      viewCount: 120,
    });

    expect(pinnedArticleView({})).toMatchObject({
      id: "",
      href: "/article/",
      title: "未命名文章",
      abstract: "这篇文章暂时没有摘要。",
      viewCount: 0,
    });
  });
});
