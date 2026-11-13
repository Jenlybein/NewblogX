import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// 顶栏搜索进到 /search 后，除了文章还要能找到用户：
// 输入作者名时，读者往往想直接进作者主页。
//
// 2026-11-13 改版：原来是「相关用户」单独一块卡片 + 下面另一个文章面板（两段式），
// 现在并成同一区域的「文章 / 用户」两个页签。这里钉住四件事——
//   1. 页面确实调用公开用户搜索接口；
//   2. 用户条目链到 /users/:id，而不是什么都不做；
//   3. 页签按 userResults 是否命中决定显不显示（不留一个恒为 0 的页签）；
//   4. 用户列表只在用户页签下渲染（不再和文章结果同时铺开）。
// 用源码守卫而不是挂载页面：search.vue 依赖 usePagedResourceCache / 路由注入，
// 挂载成本远高于这条断言要守的东西；行为正确性已由 services 与后端用例覆盖。
describe("搜索页的用户页签", () => {
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

  it("页签只在命中用户时出现", () => {
    // showUserTab 由 userResults?.length 派生；没有命中就不渲染页签栏
    expect(source).toMatch(/const showUserTab = computed\(\(\) => \(userResults\.value\?\.length \?\? 0\) > 0\)/u);
    expect(source).toContain('v-if="showUserTab"');
  });

  it("文章结果与用户列表互斥：各自只在自己那个页签下渲染", () => {
    expect(source).toContain('v-if="activeTab === \'user\'"');
    expect(source).toMatch(/<template v-else>/u);
    // 不该再有旧的"相关用户"整块标题（注释里提到历史形态是允许的）
    expect(source).not.toMatch(/section-title[^>]*>\s*相关用户/u);
  });
});
