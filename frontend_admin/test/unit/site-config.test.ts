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

describe("旧右栏配置已从后台视图模型下线", () => {
  it("手写类型 SiteRuntimeConfig 不再声明 index_right", () => {
    // 留着它等于给后台留一条"把废弃键原样写回 site 段"的通路：
    // 页面是整段读取再整段提交，类型里多一个键，保存就会把它带回去。
    expect(readApiTypes()).not.toContain("index_right");
  });

  it("站点页没有旧右栏开关控件（只有标题与启用两个字段的那套）", () => {
    const source = readSettingsSite();

    expect(source).not.toContain("site_notice");
    expect(source).not.toContain("hot_tags");
    expect(source).not.toContain("recommended_authors");
  });
});

describe("站点标题与 Logo 自定义已删除（#192）", () => {
  it("基础信息页不再有站点标题与 Logo 输入，只留备案号", () => {
    const source = readSettingsSite();

    // 这两项读者站从不消费：页头字标与浏览器标题来自品牌常量，favicon 走
    // project.icon。删掉字段后若输入框还在，保存时它只会被 JSON 忽略 ——
    // 管理员看到的是一个"改了没反应"的开关。
    expect(source).not.toContain('label="站点标题"');
    expect(source).not.toContain('label="Logo URL"');
    expect(source).not.toContain("config.site_info.title");
    expect(source).not.toContain("config.site_info.logo");
    expect(source).toContain("config.site_info.beian");
  });

  it("手写类型 SiteRuntimeConfig.site_info 只剩 beian（保存载荷不再带被删字段）", () => {
    const source = readApiTypes();
    const siteInfo = source.match(/site_info: \{([\s\S]*?)\n {2}\}/u)?.[1] ?? "";

    expect(siteInfo.length).toBeGreaterThan(0);
    // 页面是整段读取再整段提交：类型里多一个键，保存就会把它原样写回 site 段。
    expect(siteInfo).not.toMatch(/\btitle\b/u);
    expect(siteInfo).not.toMatch(/\blogo\b/u);
    expect(siteInfo).toMatch(/\bbeian: string\b/u);
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

describe("私信的陌生人开关（#202）", () => {
  it("「私信」卡里的开关绑定到 config.chat.stranger_msg_enabled", () => {
    const source = readSettingsSite();

    expect(source).toContain("私信");
    // 键名必须与后端 `conf/site.Chat` 的 json tag 一致；绑错的表现是
    // "点了保存没反应"，编译器不会说话（site 段在契约里是自由键值块）。
    expect(source).toContain("config.chat.stranger_msg_enabled");
    expect(source).toContain("允许陌生人发私信");
  });

  it("手写类型 SiteRuntimeConfig.chat 声明布尔（表单缺字段会被写成 undefined）", () => {
    const source = readApiTypes();
    const chat = source.match(/\n {2}chat: \{([\s\S]*?)\n {2}\}/u)?.[1] ?? "";

    expect(chat.length).toBeGreaterThan(0);
    expect(chat).toMatch(/\bstranger_msg_enabled: boolean/u);
  });

  it("开关是站点级策略：后台说明点明保存后立即生效，而不是要求重启", () => {
    const source = readSettingsSite();

    expect(source).toContain("站点级策略");
    expect(source).toContain("立即生效");
  });
});
