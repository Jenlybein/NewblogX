import { describe, expect, it } from "vitest";

import { sidebarCardViews } from "~/utils/sidebarCards";
import { SIDEBAR_CARDS_RENDER_CONTRACT } from "../fixtures/sidebarCardsContract";

/**
 * 读者站一侧的跨端渲染契约用例。
 *
 * 表格在 `test/fixtures/sidebarCardsContract.ts`，后台预览那一侧拿同一份字面量断言。
 * 这里只钉读者站的出口 `sidebarCardViews`：种类分派、标题回退、排序、权重、
 * 合集文案、空数据隐藏、未知种类忽略，全部由这张表决定。
 */
describe("右栏渲染模型：读者站出口与跨端契约表逐项相等", () => {
  it("同一份卡片数据经 sidebarCardViews 得到的模型与契约表完全一致", () => {
    expect(sidebarCardViews(SIDEBAR_CARDS_RENDER_CONTRACT.cards)).toEqual(
      SIDEBAR_CARDS_RENDER_CONTRACT.expected,
    );
  });
});
