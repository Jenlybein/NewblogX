import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 后台「站点」页的发布/展示开关与站点模式删除（票 #60 / ADR-0017）。
 *
 * 为什么按源码断言：后台前端没有组件测试基线（naive-ui 交互要 mock 一整套），
 * 而这些结论都只存在于 `.vue` 模板与手写类型里 —— 与 `settings-wording.test.ts`
 * 同一套做法。三条口径：
 *
 * 1. 「站点模式」整条链消失（下拉、选项、手写类型）。它是死配置：全仓没有一处
 *    读它，留着只会让管理员以为改了定位有用。
 * 2. 「发布与展示」卡片上三个开关都绑定到 `config.article.*`，键名与后端
 *    `conf/site.Article` 的 json tag 一致 —— 绑错键的表现是"点了保存没反应"，
 *    编译器不会说话（这一段在契约里是自由键值块）。
 * 3. 手写类型 `SiteRuntimeConfig` 是整段 site 配置的视图模型，缺字段会被
 *    表单静默写成 undefined。
 *
 * vitest 以 project root（frontend_admin/）为 cwd。
 */
const projectDir = process.cwd();

function readSettingsSite(): string {
  return readFileSync(join(projectDir, "app", "components", "settings", "SettingsSite.vue"), "utf8");
}

function readApiTypes(): string {
  return readFileSync(join(projectDir, "app", "types", "api.ts"), "utf8");
}

describe("「站点模式」整条链已删除（#60）", () => {
  it("后台站点页不再有下拉、选项与绑定", () => {
    const source = readSettingsSite();

    expect(source).not.toContain("站点模式");
    expect(source).not.toContain("site_info.mode");
    expect(source).not.toContain("modeOptions");
    expect(source).not.toContain("社区模式");
    expect(source).not.toContain("博客模式");
  });

  it("手写类型 SiteRuntimeConfig.site_info 不再声明 mode", () => {
    const source = readApiTypes();
    const siteInfo = source.match(/site_info: \{([\s\S]*?)\n {2}\}/u)?.[1] ?? "";

    expect(siteInfo.length).toBeGreaterThan(0);
    expect(siteInfo).not.toMatch(/\bmode\b/u);
  });
});

describe("「发布与展示」三开关（#60 / ADR-0017 决策 2）", () => {
  const SWITCHES = ["allow_non_admin_publish", "allow_non_admin_draft", "show_non_admin_articles"];

  it("同一张卡「发布与展示」里三个开关都绑定到 config.article.*", () => {
    const source = readSettingsSite();

    expect(source).toContain("发布与展示");
    for (const key of SWITCHES) {
      // 绑定写成 `config.article.<key>`：键名与后端 json tag 不一致时，
      // 保存会把一个后端不认识的键写进 site 段，表现为"点了保存没反应"。
      expect(source).toContain(`config.article.${key}`);
    }
  });

  it("三个开关的标签用 ADR-0017 的措辞（管理员读得懂关掉会发生什么）", () => {
    const source = readSettingsSite();

    expect(source).toContain("允许非管理员发布文章");
    expect(source).toContain("允许非管理员创建草稿");
    expect(source).toContain("显示非管理员发布文章");
  });

  it("手写类型 SiteRuntimeConfig.article 声明三个布尔（表单缺字段会被写成 undefined）", () => {
    const source = readApiTypes();
    const article = source.match(/\n {2}article: \{([\s\S]*?)\n {2}\}/u)?.[1] ?? "";

    expect(article.length).toBeGreaterThan(0);
    for (const key of SWITCHES) {
      expect(article).toMatch(new RegExp(`\\b${key}: boolean`, "u"));
    }
  });
});
