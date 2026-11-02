/**
 * 右栏渲染模型的**跨端契约表**（读者站与后台预览共用这一份字面量）。
 *
 * 为什么是一张共享表而不是两端各抄一遍：本票的渲染模型只有一份纯函数
 * （`frontend/app/utils/sidebarCards.ts` 的 `sidebarCardViews`），后台预览直接调用它。
 * 表锁定的是"配置 + 数据 → 渲染模型"的口径：种类分派、标题去空白与默认回退、
 * 排序号、标签云权重、合集文章数文案、空数据隐藏、未知种类忽略。
 *
 * 两端各自有一条用例拿这张表断言自己的出品：读者站断言 `sidebarCardViews`，
 * 后台断言预览出口 `sidebarCardsPreviewViews`。任一端换了渲染实现或改了字段映射，
 * 自己的用例会先红；谁也别想只改一端还保持绿。
 *
 * 卡片故意乱序（sort_order 与数组顺序不一致），并且混入未知种类与空数据卡。
 */
export const SIDEBAR_CARDS_RENDER_CONTRACT = {
  cards: [
    {
      id: "card-tag-cloud",
      kind: "tag_cloud",
      title: "",
      sort_order: 0,
      hot_articles: [],
      latest_comments: [],
      tags: [
        { id: "t1", title: "Go", article_count: 10 },
        { id: "t2", title: "Nuxt", article_count: 5 },
        { id: "t3", title: "部署", article_count: 1 },
      ],
      collections: [],
    },
    {
      id: "card-latest-comments",
      kind: "latest_comments",
      title: "大家怎么说",
      sort_order: 1,
      hot_articles: [],
      latest_comments: [
        {
          id: "c1",
          content: "写得很清楚",
          article_id: "a1",
          article_title: "文章一",
          user_nickname: "读者甲",
          created_at: "2026-11-02T00:00:00Z",
        },
      ],
      tags: [],
      collections: [],
    },
    {
      id: "card-hot-articles",
      kind: "hot_articles",
      title: " 本周热读 ",
      sort_order: 2,
      hot_articles: [
        { id: "a1", title: "热读一", digg_count: 12, view_count: 120 },
        { id: "a2", title: "热读二", digg_count: 3, view_count: 30 },
      ],
      latest_comments: [],
      tags: [],
      collections: [],
    },
    {
      id: "card-collections",
      kind: "recommended_collections",
      title: "往期专题",
      sort_order: 3,
      hot_articles: [],
      latest_comments: [],
      tags: [],
      collections: [
        {
          id: "101",
          title: "从零搭一个博客",
          description: "简介 101",
          cover: "https://cdn.example.com/101.png",
          article_count: 7,
        },
        {
          id: "102",
          title: "  ",
          description: "  ",
          cover: "",
          article_count: 0,
        },
      ],
    },
    {
      id: "card-unknown",
      kind: "ai_card",
      title: "AI 入口",
      sort_order: 4,
      hot_articles: [{ id: "a9", title: "不该出现", digg_count: 99, view_count: 990 }],
      latest_comments: [],
      tags: [],
      collections: [],
    },
    {
      id: "card-empty",
      kind: "tag_cloud",
      title: "空标签云",
      sort_order: 5,
      hot_articles: [],
      latest_comments: [],
      tags: [],
      collections: [],
    },
  ],
  expected: [
    {
      key: "card-tag-cloud",
      kind: "tag_cloud",
      title: "标签云",
      sortOrder: 0,
      articles: [],
      comments: [],
      tags: [
        { id: "t1", title: "Go", articleCount: 10, weight: 1 },
        { id: "t2", title: "Nuxt", articleCount: 5, weight: 0.5 },
        { id: "t3", title: "部署", articleCount: 1, weight: 0.1 },
      ],
      collections: [],
      contentHtml: "",
    },
    {
      key: "card-latest-comments",
      kind: "latest_comments",
      title: "大家怎么说",
      sortOrder: 1,
      articles: [],
      comments: [
        {
          id: "c1",
          content: "写得很清楚",
          articleId: "a1",
          articleTitle: "文章一",
          userNickname: "读者甲",
          createdAt: "2026-11-02T00:00:00Z",
        },
      ],
      tags: [],
      collections: [],
      contentHtml: "",
    },
    {
      key: "card-hot-articles",
      kind: "hot_articles",
      title: "本周热读",
      sortOrder: 2,
      articles: [
        { id: "a1", title: "热读一", diggCount: 12, viewCount: 120 },
        { id: "a2", title: "热读二", diggCount: 3, viewCount: 30 },
      ],
      comments: [],
      tags: [],
      collections: [],
      contentHtml: "",
    },
    {
      key: "card-collections",
      kind: "recommended_collections",
      title: "往期专题",
      sortOrder: 3,
      articles: [],
      comments: [],
      tags: [],
      collections: [
        {
          id: "101",
          href: "/collections/101",
          title: "从零搭一个博客",
          description: "简介 101",
          cover: "https://cdn.example.com/101.png",
          countLabel: "7 篇文章",
        },
        {
          id: "102",
          href: "/collections/102",
          title: "未命名合集",
          description: "",
          cover: "",
          countLabel: "暂无文章",
        },
      ],
      contentHtml: "",
    },
  ],
};
