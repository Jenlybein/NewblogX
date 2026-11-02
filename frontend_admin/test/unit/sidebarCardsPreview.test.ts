import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import type { SidebarCardDraft } from "~/utils/sidebarCardsAdmin";
import {
  SIDEBAR_PREVIEW_WIDTH_PX,
  sidebarCardsPreviewItems,
  sidebarCardsPreviewViews,
} from "~/utils/sidebarCardsPreview";
import { sidebarCardViews } from "../../../frontend/app/utils/sidebarCards";
import { SIDEBAR_CARDS_RENDER_CONTRACT } from "../../../frontend/test/fixtures/sidebarCardsContract";

/**
 * 后台右栏预览的纯函数层。
 *
 * 主缝是"草稿 → 预览模型"：不挂载组件、不调保存接口，直接断言未保存的编辑
 * 已经体现在模型里。跨端口径用同一张契约表（读者站那边也拿这份字面量）。
 */

const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");
const previewComponent = join(projectDir, "app/components/settings/SidebarCardsPreview.vue");

function draft(
  key: string,
  kind: string,
  title: string,
  enabled = true,
  id = "",
): SidebarCardDraft {
  return { key, id, kind, title, enabled };
}

/** 从读者站首页两栏栅格（`.hero-grid` / `.content-grid`）取出右栏固定宽度。 */
function readerSidebarWidths(): number[] {
  const css = readFileSync(join(repoRoot, "frontend/app/assets/css/tailwind.css"), "utf8");
  return ["hero-grid", "content-grid"].map((className) => {
    const pattern = new RegExp(
      `\\.${className}\\s*\\{[^}]*grid-cols-\\[minmax\\(0,1fr\\)_([0-9]+)px\\]`,
      "u",
    );
    const match = pattern.exec(css);
    if (!match) throw new Error(`读者站样式里找不到 .${className} 的右栏固定宽度`);
    return Number(match[1]);
  });
}

describe("右栏预览：宽度等于首页右栏固定宽度", () => {
  it("预览宽度常量等于读者站两栏栅格里的右栏真实宽度，不模拟断点", () => {
    // 首页右栏在 `.hero-grid` / `.content-grid` 里是同一个固定宽度。
    const widths = readerSidebarWidths();
    expect(widths).toEqual([SIDEBAR_PREVIEW_WIDTH_PX, SIDEBAR_PREVIEW_WIDTH_PX]);
    expect(SIDEBAR_PREVIEW_WIDTH_PX).toBe(310);
  });
});

describe("右栏预览：草稿改动即时反映在模型里（不需要保存）", () => {
  it("改标题立刻出现在预览模型里，且没有回读服务端", () => {
    const drafts = [
      draft("card-11", "hot_articles", "热门文章", true, "11"),
      draft("card-12", "latest_comments", "最新评论", true, "12"),
    ];

    const before = sidebarCardsPreviewViews(sidebarCardsPreviewItems(drafts));
    expect(before.map(view => view.title)).toEqual(["热门文章", "最新评论"]);

    const renamed = drafts.map(item =>
      item.key === "card-11" ? { ...item, title: "大家在读" } : item,
    );
    const after = sidebarCardsPreviewViews(sidebarCardsPreviewItems(renamed));

    expect(after.map(view => view.title)).toEqual(["大家在读", "最新评论"]);
    // 预览只吃本地草稿，不需要保存/回读。
    expect(after.map(view => view.kind)).toEqual(["hot_articles", "latest_comments"]);
  });

  it("拖动顺序立刻反映；停用卡从预览消失但草稿里仍留着", () => {
    const drafts = [
      draft("card-11", "hot_articles", "热门文章", true, "11"),
      draft("card-12", "tag_cloud", "标签云", true, "12"),
    ];
    const moved = [drafts[1]!, drafts[0]!];
    expect(sidebarCardsPreviewViews(sidebarCardsPreviewItems(moved)).map(view => view.kind)).toEqual([
      "tag_cloud",
      "hot_articles",
    ]);

    const disabled = draft("card-12", "tag_cloud", "标签云", false, "12");
    expect(
      sidebarCardsPreviewViews(sidebarCardsPreviewItems([drafts[0]!, disabled])).map(view => view.kind),
    ).toEqual(["hot_articles"]);
  });

  it("新增卡立即出现在预览里：新卡没有服务端数据，用该种类的示例内容撑起版面", () => {
    const drafts = [
      draft("new-1", "hot_articles", "", true),
      draft("new-2", "latest_comments", "   ", true),
      draft("new-3", "tag_cloud", "标签云", true),
      draft("new-4", "recommended_collections", "推荐合集", true),
    ];

    const views = sidebarCardsPreviewViews(sidebarCardsPreviewItems(drafts));
    expect(views.map(view => [view.kind, view.title])).toEqual([
      ["hot_articles", "热门文章"],
      ["latest_comments", "最新评论"],
      ["tag_cloud", "标签云"],
      ["recommended_collections", "推荐合集"],
    ]);

    // 每种卡都带着能渲染的示例数据（否则空数据卡会被整块隐藏，预览变成空白）。
    expect(views[0]!.articles.length).toBeGreaterThan(0);
    expect(views[1]!.comments.length).toBeGreaterThan(0);
    expect(views[2]!.tags.length).toBeGreaterThan(0);
    expect(views[3]!.collections.length).toBeGreaterThan(0);

    // 排序号取草稿下标，与读者接口的 sort_order 口径一致。
    expect(sidebarCardsPreviewItems(drafts).map(item => item.sort_order)).toEqual([0, 1, 2, 3]);
    // 新卡没有服务端 id，用本地 key 生成稳定预览标识。
    expect(sidebarCardsPreviewItems(drafts).map(item => item.id)).toEqual([
      "preview-new-1",
      "preview-new-2",
      "preview-new-3",
      "preview-new-4",
    ]);
  });

  it("未知种类不进预览（与读者站一样整张忽略，不画空盒子）", () => {
    const views = sidebarCardsPreviewViews(
      sidebarCardsPreviewItems([draft("new-9", "ai_card", "AI 入口")]),
    );
    expect(views).toEqual([]);
  });

  it("富文本卡写了正文就能进预览：正文折成纯文本且保留分段", () => {
    const richDraft = {
      ...draft("new-rich", "rich_text", "公告", true),
      content: "<h2>更新日志</h2><p>本周修了三个问题。</p><p>下周计划见下。</p>",
    };

    const views = sidebarCardsPreviewViews(sidebarCardsPreviewItems([richDraft]));
    expect(views).toHaveLength(1);
    const view = views[0]!;
    expect(view.kind).toBe("rich_text");

    // 标签被折掉，但段落边界变成换行 —— 预览里看得出"分了几段"。
    expect(view.contentHtml).not.toContain("<");
    expect(view.contentHtml).toContain("更新日志");
    expect(view.contentHtml).toContain("本周修了三个问题。");
    expect(view.contentHtml.split("\n").filter(Boolean)).toEqual([
      "更新日志",
      "本周修了三个问题。",
      "下周计划见下。",
    ]);
  });

  it("富文本卡正文为空时整张不进预览（与读者站同一条空数据规则）", () => {
    const views = sidebarCardsPreviewViews(
      sidebarCardsPreviewItems([{ ...draft("new-empty", "rich_text", "公告", true), content: "   " }]),
    );
    expect(views).toEqual([]);
  });
});

describe("右栏预览：与读者站同一张契约表、同一份渲染函数", () => {
  it("后台预览出口与读者站出口对同一份卡片数据逐项相等", () => {
    const cards = SIDEBAR_CARDS_RENDER_CONTRACT.cards;
    const preview = sidebarCardsPreviewViews(cards);
    const reader = sidebarCardViews(cards);

    expect(preview).toEqual(reader);
    // 同一张逐字契约表：谁改了字段映射，谁自己的用例先红。
    expect(preview).toEqual(SIDEBAR_CARDS_RENDER_CONTRACT.expected);
  });
});

describe("右栏预览：不做骨架屏", () => {
  it("预览组件源码不引入骨架屏，加载态也不画占位块", () => {
    const source = readFileSync(previewComponent, "utf8");
    expect(source).not.toMatch(/NSkeleton|n-skeleton|skeleton/iu);
  });
});
