import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// 顶栏搜索进到 /search 后，除了文章还要有一栏「相关用户」：
// 输入作者名时，读者往往想直接进作者主页。这里钉住三件事——
//   1. 页面确实调用公开用户搜索接口；
//   2. 用户条目链到 /users/:id，而不是什么都不做；
//   3. 空结果时不渲染这一栏（不留一个空壳标题）。
// 用源码守卫而不是挂载页面：search.vue 依赖 usePagedResourceCache / 路由注入，
// 挂载成本远高于这条断言要守的东西；行为正确性已由 services 与后端用例覆盖。
describe("搜索页的用户分栏", () => {
  const source = readFileSync(resolve(process.cwd(), "app/pages/search.vue"), "utf8");

  it("调用公开用户搜索服务", () => {
    expect(source).toContain('from "~/services/user"');
    expect(source).toContain("searchUsers({ key: keyword");
  });

  it("用户条目链到公开主页，且有头像兜底", () => {
    expect(source).toContain('class="search-user-card"');
    expect(source).toContain(":to=\"`/users/${user.id}`\"");
    expect(source).toContain("search-user-card__avatar--fallback");
  });

  it("没有命中用户时整栏不出现", () => {
    expect(source).toMatch(/v-if="userResults\?\.length"/u);
  });
});
