import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const projectDir = process.cwd();
const searchPage = readFileSync(resolve(projectDir, "app/pages/search.vue"), "utf8");

/**
 * 搜索页的标签筛选用「发布文章」那套 chip 网格，而不是原生 `<select>`。
 *
 * 背景：标签词库几十条，原生下拉是一行一个的竖向列表，翻着找是发布流程里的
 * 老痛点；搜索页当初照抄了原生下拉，于是同一个词库在两处有两种找法。
 * 这里只锁"用同一个组件 + 走同一条 v-model 语义"，样式由组件自己负责。
 */
describe("搜索页标签菜单", () => {
  it("用 ChipSelect（和发布对话框同一个组件），不是原生 select", () => {
    expect(searchPage).toContain('from "~/components/common/ChipSelect.vue"');
    expect(searchPage).toContain("<ChipSelect");
    expect(searchPage).toContain('data-role="search-tag"');
  });

  it("标签不再挂在原生 select 上（排序仍是原生下拉，不在本次范围）", () => {
    expect(searchPage).not.toMatch(/<select[^>]*name="tag_ids"/u);
    expect(searchPage).not.toContain('aria-label="标签筛选"');
    // 排序保持原样：它是短列表，原生下拉没有翻找成本。
    expect(searchPage).toMatch(/<select[^>]*name="sort"/u);
  });

  it("单选语义：清空回到空串，查询参数与 URL 同步逻辑不用改", () => {
    expect(searchPage).toContain("const tagModel = computed<string | null>");
    expect(searchPage).toMatch(/tagId\.value \|\| null/u);
    expect(searchPage).toMatch(/tagId\.value = typeof value === "string" \? value : ""/u);
    // 选项值必须归一成字符串，chip 组件的泛型是 string。
    expect(searchPage).toMatch(/value: String\(option\.value\)/u);
  });
});
