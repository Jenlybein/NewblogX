import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 页面取数编排门禁（ADR-0019 决策 3 / 约束 6）。
 *
 * 页面 setup 不得写成"await useAsyncData → await useAsyncData → …"的串行水车：
 * 每个 await 都把上一段网络往返串进下一步，首屏/切页的总时长等于最慢的接口。
 * 要求是：多个源**并行发起**（不 await），页面先出骨架（`LoadingSkeleton`）。
 *
 * 这里锁两个曾经违反约束的页面：`/users/[id]`（个人主页）与 `/search`（搜索页）。
 * 页面组件依赖 Nuxt 自动导入，测试环境 mount 不起来，所以用源码结构判据 +
 * 人造违规 canary（判据能抓到"串行 await 被写回去"），风格参照 test/gates/rendering-boundary.test.ts。
 */
const projectDir = process.cwd();

const usersPage = readFileSync(join(projectDir, "app/pages/users/[id].vue"), "utf8");
const searchPage = readFileSync(join(projectDir, "app/pages/search.vue"), "utf8");

/** 串行阻塞 setup 的取数调用数：`await useAsyncData(...)` 与 `await useArticleSearch(...)`。 */
function serialBlockingFetches(source: string): number {
  const patterns = [/await\s+useAsyncData/gu, /await\s+useArticleSearch/gu];
  return patterns.reduce((count, pattern) => count + (source.match(pattern)?.length ?? 0), 0);
}

describe("页面取数不得串行阻塞 setup（ADR-0019 约束 6）", () => {
  it("个人主页：不再有 await useAsyncData / await useArticleSearch 的串行链", () => {
    expect(serialBlockingFetches(usersPage)).toBe(0);
  });

  it("个人主页：资料与文章流都先出骨架（LoadingSkeleton + pending 可观察）", () => {
    expect(usersPage).toContain("LoadingSkeleton");
    expect(usersPage).toContain("profilePending");
    expect(usersPage).toContain("articlePending");
  });

  it("搜索页：标签不再先 await 再建分页器，结果区改成 background 首屏 + 骨架", () => {
    expect(serialBlockingFetches(searchPage)).toBe(0);
    expect(searchPage).toContain('initialLoad: "background"');
    expect(searchPage).toContain("LoadingSkeleton");
    expect(searchPage).toContain("pending && !articles.length");
  });

  it("判据不是恒绿：把串行 await 写回去会红（canary）", () => {
    const canary = [
      "const { data: a } = await useAsyncData(() => 'a', fetchA);",
      "const { articles } = await useArticleSearch(query, { lazy: true });",
    ].join("\n");

    expect(serialBlockingFetches(canary)).toBe(2);
  });
});
