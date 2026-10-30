import { describe, expect, it } from "vitest";

import type { FavoriteArticleItem, SearchArticleItem } from "~/types/api";
import {
  EMPTY_ARTICLE_ENTRY,
  fromFavoriteArticle,
  fromSearchArticle,
  type ArticleEntryData,
} from "~/utils/articleEntry";

/** 契约字段全部可选，测试里只给用得到的字段，用一次显式断言收口。 */
function searchArticle(overrides: Record<string, unknown>): SearchArticleItem {
  return overrides as unknown as SearchArticleItem;
}

function favoriteArticle(overrides: Record<string, unknown>): FavoriteArticleItem {
  return overrides as unknown as FavoriteArticleItem;
}

describe("articleEntry：文章卡片统一视图模型", () => {
  it("列表/搜索数据：标题、摘要、标签、统计、作者都归一化", () => {
    const entry = fromSearchArticle(searchArticle({
      id: "a1",
      title: "Nuxt 部署笔记",
      abstract: "讲清楚 SSR 与静态生成的选择。",
      cover: " https://image.example.com/cover.png ",
      created_at: "2026-10-01T10:00:00Z",
      category: { title: "工程" },
      author: { id: "u1", nickname: "作者甲", avatar: "https://image.example.com/u1.png" },
      tags: [
        { id: "t1", title: "Nuxt" },
        { id: "t2", title: " " },
        { id: "t3", title: "部署" },
      ],
      digg_count: 12,
      favor_count: 34,
      comment_count: 5,
      view_count: 1200,
    }));

    expect(entry.id).toBe("a1");
    expect(entry.href).toBe("/article/a1");
    expect(entry.title).toBe("Nuxt 部署笔记");
    expect(entry.abstract).toBe("讲清楚 SSR 与静态生成的选择。");
    expect(entry.cover).toBe("https://image.example.com/cover.png");
    expect(entry.categoryLabel).toBe("工程");
    expect(entry.author).toEqual({
      id: "u1",
      nickname: "作者甲",
      avatar: "https://image.example.com/u1.png",
    });
    expect(entry.tags.map((tag) => tag.title)).toEqual(["Nuxt", "部署"]);
    expect(entry.stats).toEqual({ digg: 12, favor: 34, comment: 5, view: 1200 });
    expect(entry.shelfMeta).toEqual([]);
    expect(entry.dateLabel).not.toBe("");
  });

  it("命中高亮时优先展示高亮标题/摘要；缺字段时给可读兜底", () => {
    const entry = fromSearchArticle(searchArticle({
      id: "a2",
      title: "原标题",
      abstract: "",
      highlight: { title: "搜索命中：原标题", abstract: "命中的摘要片段" },
    }));

    expect(entry.title).toBe("搜索命中：原标题");
    expect(entry.abstract).toBe("命中的摘要片段");
    expect(entry.author).toBeNull();
    expect(entry.tags).toEqual([]);
    expect(entry.stats).toEqual({ digg: 0, favor: 0, comment: 0, view: 0 });
    expect(entry.statusLabel).toBe("");
  });

  it("收藏架数据：与列表同一形状，多一行收藏元信息，作者无 ID 时不生成主页链接", () => {
    const entry = fromFavoriteArticle(favoriteArticle({
      article_id: "a3",
      title: "被收藏的文章",
      abstract: "收藏夹里的文章摘要。",
      cover: "https://image.example.com/cover3.png",
      user_nickname: "原作者",
      user_avatar: "https://image.example.com/u3.png",
      article_status: 3,
      favorited_at: "2026-10-05T12:30:00Z",
      digg_count: 1,
      favor_count: 2,
      comment_count: 3,
      view_count: 40,
    }));

    expect(entry.id).toBe("a3");
    expect(entry.title).toBe("被收藏的文章");
    expect(entry.statusLabel).toBe("已发布");
    expect(entry.author).toEqual({ id: "", nickname: "原作者", avatar: "https://image.example.com/u3.png" });
    expect(entry.tags).toEqual([]);
    expect(entry.shelfMeta).toEqual(["40 阅读", "2 收藏"]);
    expect(entry.dateLabel).not.toBe("");
  });

  it("收藏架数据：未发布状态与空标题/摘要的降级", () => {
    const entry = fromFavoriteArticle(favoriteArticle({
      article_id: "a4",
      title: "",
      abstract: "",
      article_status: 1,
      view_count: 0,
      favor_count: 0,
    }));

    expect(entry.title).toBe("未命名文章");
    expect(entry.abstract).toBe("这篇文章暂时没有摘要。");
    expect(entry.statusLabel).toBe("状态 1");
    expect(entry.author).toBeNull();
    expect(entry.shelfMeta).toEqual(["0 阅读", "0 收藏"]);
  });

  it("空条目常量本身可安全渲染（供卡片在无数据时兜底）", () => {
    const view: ArticleEntryData = EMPTY_ARTICLE_ENTRY;
    expect(view.title).toBe("未命名文章");
    expect(view.abstract).toBe("这篇文章暂时没有摘要。");
    expect(view.tags).toEqual([]);
    expect(view.author).toBeNull();
    expect(view.stats.view).toBe(0);
  });
});
