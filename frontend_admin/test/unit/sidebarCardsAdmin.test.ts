import { describe, expect, it } from "vitest";
import {
  SIDEBAR_CARD_DEFAULT_TITLES,
  SIDEBAR_CARD_KINDS,
  SIDEBAR_CARD_MAX_COUNT,
  addSidebarCardDraft,
  effectiveSidebarCardTitle,
  moveSidebarCardDraft,
  removeSidebarCardDraft,
  renameSidebarCardDraft,
  sidebarCardDraftErrors,
  sidebarCardDrafts,
  toSidebarCardsSavePayload,
  toggleSidebarCardDraft,
} from "~/utils/sidebarCardsAdmin";
import type { SidebarCardDraft } from "~/utils/sidebarCardsAdmin";
import type {
  AdminSidebarCardItem,
  SidebarCardsSavePayload,
  SiteSidebarCardItem,
} from "~/types/api";
import { sidebarCardViews } from "../../../frontend/app/utils/sidebarCards";

/**
 * 后台右栏卡片管理器的纯函数层。
 *
 * 五类操作（新增 / 删除 / 拖动排序 / 改标题 / 启停）都在这里收口，
 * 页面与组件只做接线。顺序相关用例不只断言本地数组，还走一条假后端：
 * 保存负载 → 按后端口径（顺序号取下标、标题空白回退、停用项保留）
 * → 读者侧纯函数渲染，验证后台拖出来的顺序就是读者看到的顺序。
 */

/** 一排服务端卡片，字段与契约一致。 */
function serverCard(
  id: string,
  kind: string,
  title: string,
  sortOrder: number,
  enabled: boolean,
): AdminSidebarCardItem {
  return { id, kind, title, sort_order: sortOrder, enabled };
}

/** 三张初始卡：热门文章、最新评论（停用）、标签云。 */
function initialRows(): AdminSidebarCardItem[] {
  return [
    serverCard("11", "hot_articles", "热门文章", 0, true),
    serverCard("12", "latest_comments", "", 1, false),
    serverCard("13", "tag_cloud", "标签云", 2, true),
  ];
}

/**
 * 假后端：整表保存的服务端口径。
 *
 * 与 `internal/site/app/sidebar.go` 对齐：顺序号由提交顺序重排成 0..n-1，
 * 标题去空白后为空则回退默认标题，停用项照旧落库（不是物理删除）。
 */
function fakeSave(payload: SidebarCardsSavePayload): AdminSidebarCardItem[] {
  return payload.list.map((item, index) => ({
    id: String(100 + index),
    kind: item.kind,
    title: item.title.trim() || SIDEBAR_CARD_DEFAULT_TITLES[item.kind as keyof typeof SIDEBAR_CARD_DEFAULT_TITLES],
    sort_order: index,
    enabled: item.enabled,
  }));
}

/** 假读者接口：只下发启用项，按顺序号排，并给每种卡配一条数据。 */
function fakeReader(rows: AdminSidebarCardItem[]): SiteSidebarCardItem[] {
  return rows
    .filter(row => row.enabled)
    .sort((left, right) => left.sort_order - right.sort_order)
    .map(row => ({
      id: row.id,
      kind: row.kind,
      title: row.title,
      sort_order: row.sort_order,
      hot_articles:
        row.kind === "hot_articles"
          ? [{ id: "a1", title: "热文", digg_count: 3, view_count: 9 }]
          : [],
      latest_comments:
        row.kind === "latest_comments"
          ? [
              {
                id: "c1",
                content: "评论",
                article_id: "1",
                article_title: "文章",
                user_nickname: "读者",
                created_at: "2026-01-01T00:00:00Z",
              },
            ]
          : [],
      tags: row.kind === "tag_cloud" ? [{ id: "t1", title: "Go", article_count: 5 }] : [],
    }));
}

/** 按 key 找草稿，找不到直接让用例失败（比断言可空链更清楚）。 */
function draftOf(drafts: SidebarCardDraft[], key: string): SidebarCardDraft {
  const found = drafts.find(draft => draft.key === key);
  if (!found) throw new Error(`草稿里没有 key=${key}`);
  return found;
}

/** 草稿列表的种类顺序，用来断言拖动结果。 */
function kindsOf(drafts: SidebarCardDraft[]): string[] {
  return drafts.map(draft => draft.kind);
}

describe("右栏卡片管理器：加载整张配置表", () => {
  it("停用卡也读进来、保持运营顺序，标题空白时按后端口径回退默认标题", () => {
    const drafts = sidebarCardDrafts(initialRows());

    expect(drafts.map(draft => draft.kind)).toEqual([
      "hot_articles",
      "latest_comments",
      "tag_cloud",
    ]);
    expect(drafts.map(draft => draft.enabled)).toEqual([true, false, true]);
    expect(drafts.map(draft => draft.id)).toEqual(["11", "12", "13"]);

    // 停用项不是"消失"，只是读者侧不渲染；管理端必须能继续编辑。
    expect(drafts[1]!.title).toBe("");
    expect(effectiveSidebarCardTitle(drafts[1]!.kind, drafts[1]!.title)).toBe("最新评论");
    expect(effectiveSidebarCardTitle("tag_cloud", "  标签云  ")).toBe("标签云");
  });

  it("未知种类保留在列表里（不静默丢弃），空输入返回空列表", () => {
    const drafts = sidebarCardDrafts([
      serverCard("1", "hot_articles", "热门", 0, true),
      serverCard("2", "friend_links", "友情链接", 1, true),
    ]);

    expect(drafts.map(draft => draft.kind)).toEqual(["hot_articles", "friend_links"]);
    expect(sidebarCardDraftErrors(drafts)).toContain(
      "第 2 张卡的种类「friend_links」不受支持",
    );
    expect(sidebarCardDrafts(null)).toEqual([]);
    expect(sidebarCardDrafts([])).toEqual([]);
  });
});

describe("右栏卡片管理器：新增 / 删除 / 改标题 / 启停", () => {
  it("新增追加到末尾、默认启用且标题留空（交给默认标题回退）", () => {
    const drafts = addSidebarCardDraft(sidebarCardDrafts(initialRows()), "tag_cloud");

    expect(drafts).toHaveLength(4);
    expect(drafts[3]).toMatchObject({
      id: "",
      kind: "tag_cloud",
      title: "",
      enabled: true,
    });
    // 新卡与已有卡的 key 不冲突。
    expect(new Set(drafts.map(draft => draft.key)).size).toBe(4);
  });

  it("到达上限后不再追加，并给出与后端一致的条数错误", () => {
    let drafts = sidebarCardDrafts([]);
    for (let index = 0; index < SIDEBAR_CARD_MAX_COUNT; index += 1) {
      drafts = addSidebarCardDraft(drafts, SIDEBAR_CARD_KINDS[index % SIDEBAR_CARD_KINDS.length]!);
    }
    expect(drafts).toHaveLength(SIDEBAR_CARD_MAX_COUNT);

    const overflow = addSidebarCardDraft(drafts, "tag_cloud");
    expect(overflow).toHaveLength(SIDEBAR_CARD_MAX_COUNT);
    expect(sidebarCardDraftErrors(overflow)).toEqual([]);

    // 后端在超过上限时整表拒绝，这里能表达同一口径（真实超限只可能来自历史数据）。
    const tooMany = [...overflow, { ...overflow[0]!, key: "new-99" }];
    expect(sidebarCardDraftErrors(tooMany)).toEqual([
      `最多 ${SIDEBAR_CARD_MAX_COUNT} 张卡片，当前 ${SIDEBAR_CARD_MAX_COUNT + 1} 张`,
    ]);
  });

  it("删除按 key 精确移除；改标题只动那一张，保存负载会去掉首尾空白", () => {
    let drafts = sidebarCardDrafts(initialRows());
    const target = drafts[1]!;

    drafts = removeSidebarCardDraft(drafts, target.key);
    expect(kindsOf(drafts)).toEqual(["hot_articles", "tag_cloud"]);
    // 未知 key 不误删。
    expect(removeSidebarCardDraft(drafts, "not-there")).toHaveLength(drafts.length);

    const hotKey = drafts[0]!.key;
    drafts = renameSidebarCardDraft(drafts, hotKey, "  本周热读  ");
    expect(draftOf(drafts, hotKey).title).toBe("  本周热读  ");

    const payload = toSidebarCardsSavePayload(drafts);
    expect(payload.list[0]!.title).toBe("本周热读");
    // 标题清空时保持空白，交给后端默认标题回退，而不是前端替它写死。
    drafts = renameSidebarCardDraft(drafts, hotKey, "   ");
    expect(toSidebarCardsSavePayload(drafts).list[0]!.title).toBe("");
  });

  it("启停按 key 翻转，也能显式置位；停用项始终留在保存负载里", () => {
    let drafts = sidebarCardDrafts(initialRows());
    const hotKey = drafts[0]!.key;

    drafts = toggleSidebarCardDraft(drafts, hotKey);
    expect(draftOf(drafts, hotKey).enabled).toBe(false);
    drafts = toggleSidebarCardDraft(drafts, hotKey);
    expect(draftOf(drafts, hotKey).enabled).toBe(true);
    drafts = toggleSidebarCardDraft(drafts, hotKey, false);
    expect(draftOf(drafts, hotKey).enabled).toBe(false);

    const payload = toSidebarCardsSavePayload(drafts);
    expect(payload.list).toHaveLength(3);
    expect(payload.list[1]!.enabled).toBe(false);
  });
});

describe("右栏卡片管理器：拖动排序与落库顺序", () => {
  it("拖动重排后保存，整表的顺序号按新顺序连续重排成 0..n-1", () => {
    let drafts = sidebarCardDrafts(initialRows());
    // 把第一张拖到最后。
    drafts = moveSidebarCardDraft(drafts, 0, 2);
    expect(kindsOf(drafts)).toEqual(["latest_comments", "tag_cloud", "hot_articles"]);

    const payload = toSidebarCardsSavePayload(drafts);
    expect(payload.list.map(item => item.kind)).toEqual(kindsOf(drafts));
    // 契约里没有 sort_order：顺序的唯一权威就是 list 的下标。
    expect(payload.list.every(item => !("sort_order" in item))).toBe(true);

    const saved = fakeSave(payload);
    expect(saved.map(row => row.sort_order)).toEqual([0, 1, 2]);
    expect(saved.map(row => row.kind)).toEqual(kindsOf(drafts));
  });

  it("越界 / 原地拖动不改变顺序，且原数组不被修改", () => {
    const drafts = sidebarCardDrafts(initialRows());
    const snapshot = kindsOf(drafts);

    expect(kindsOf(moveSidebarCardDraft(drafts, 0, 0))).toEqual(snapshot);
    expect(kindsOf(moveSidebarCardDraft(drafts, -1, 1))).toEqual(snapshot);
    expect(kindsOf(moveSidebarCardDraft(drafts, 1, 99))).toEqual(snapshot);
    expect(kindsOf(drafts)).toEqual(snapshot);
  });
});

describe("右栏卡片管理器：后台顺序与读者站一致", () => {
  it("后台拖出的顺序经过整表保存后，读者渲染顺序逐项一致", () => {
    let drafts = sidebarCardDrafts(initialRows());
    // 全部启用：本用例只看顺序，停用过滤由下一条用例覆盖。
    drafts = drafts.map(draft => ({ ...draft, enabled: true }));
    // 标签云拖到最前。
    drafts = moveSidebarCardDraft(drafts, 2, 0);
    const saved = fakeSave(toSidebarCardsSavePayload(drafts));
    expect(saved.map(row => row.sort_order)).toEqual([0, 1, 2]);

    const views = sidebarCardViews(fakeReader(saved));
    expect(views.map(view => view.kind)).toEqual(kindsOf(drafts));
    expect(views.map(view => view.title)).toEqual(["标签云", "热门文章", "最新评论"]);
  });

  it("停用卡不进读者侧，但仍留在管理端配置与保存负载里", () => {
    let drafts = sidebarCardDrafts(initialRows());
    const hotKey = drafts[0]!.key;
    drafts = toggleSidebarCardDraft(drafts, hotKey, false);

    const payload = toSidebarCardsSavePayload(drafts);
    // 保存负载包含停用项 —— 整表保存不是"只提交启用项"。
    expect(payload.list.map(item => item.enabled)).toEqual([false, false, true]);

    const saved = fakeSave(payload);
    expect(saved).toHaveLength(3);
    expect(saved[0]).toMatchObject({ kind: "hot_articles", enabled: false, sort_order: 0 });

    const views = sidebarCardViews(fakeReader(saved));
    expect(views.map(view => view.kind)).toEqual(["tag_cloud"]);
    // 配置仍在：重新读取管理端列表时它还在原位。
    expect(sidebarCardDrafts(saved).map(draft => draft.kind)).toEqual([
      "hot_articles",
      "latest_comments",
      "tag_cloud",
    ]);
  });

  it("修改标题后读者侧显示新标题", () => {
    let drafts = sidebarCardDrafts(initialRows());
    const hotKey = drafts[0]!.key;
    drafts = renameSidebarCardDraft(drafts, hotKey, "大家在读");

    const views = sidebarCardViews(fakeReader(fakeSave(toSidebarCardsSavePayload(drafts))));
    expect(views.find(view => view.kind === "hot_articles")?.title).toBe("大家在读");
  });
});
