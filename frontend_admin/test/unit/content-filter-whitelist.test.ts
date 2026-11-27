import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 关键词过滤页按新一代设计稿重排（2026-11-27）。
 *
 * 这一票同时做两件事：词表页整页重排 + 白名单拆独立页。后台没有组件测试基线
 * （naive-ui 交互要 mock 一整套），所以沿用 settings-wording.test.ts 的做法按源码断言；
 * 真正会算错的逻辑（打星演示、保存字段归属）抽成了纯函数，另有行为断言见
 * `content-filter-layout.test.ts`。
 *
 * 这里钉三件评审里被点出来的事：
 *
 * 1. 白名单搬到独立组件/页面，短语区必须真的绑 `whitelist_phrases`（不是只改文案）；
 * 2. Tab 计数必须用后端下发的 `whitelist_stats.total`（前端自己把两个数组相加
 *    会出现"页面 50、体检 49"）；
 * 3. 边界说明必须在（短语只覆盖已知连续写法、长短语会连带豁免其中其它敏感词）——
 *    上一版用"越长越安全"顶替了精确说明，运营会以为它能自动覆盖变体。
 */
const projectDir = process.cwd();
const settingsDir = join(projectDir, "app", "components", "settings");

function readSettings(name: string): string {
  return readFileSync(join(settingsDir, name), "utf8");
}

function readWordsPage(): string {
  return readSettings("SettingsContentFilter.vue");
}

function readWhitelistPage(): string {
  return readSettings("SettingsContentFilterWhitelist.vue");
}

describe("白名单拆页（ADR-0032 补丁四 / 2026-11-27 重排）", () => {
  it("短语区绑定 whitelist_phrases，页面级保存按钮在", () => {
    const source = readWhitelistPage();

    expect(source).toContain("whitelist_phrases");
    expect(source).toContain("保存白名单");
    // 短语与词级两份都要能读、能写。
    expect(source).toContain("whitelist.value");
    expect(source).toContain("whitelistPhrases.value");
  });

  it("Tab 计数用后端口径，不自己把两个数组相加", () => {
    for (const source of [readWordsPage(), readWhitelistPage()]) {
      expect(source).toContain("whitelist_stats");
      expect(source).not.toContain("whitelist.length + whitelistPhrases.length");
    }
  });

  it("写清短语的边界，不用「越长越安全」笼统带过", () => {
    const source = readWhitelistPage();

    expect(source).toContain("完整包住");
    expect(source).toContain("已知的连续写法");
    expect(source).toContain("连带");
    expect(source).not.toContain("越长越安全");
  });

  it("两页各自只覆盖自己拥有的字段（白名单页不碰词表）", () => {
    const words = readWordsPage();
    const whitelistPage = readWhitelistPage();

    expect(words).toContain("CONTENT_FILTER_WORD_FIELDS");
    expect(words).not.toContain("CONTENT_FILTER_WHITELIST_FIELDS");
    expect(whitelistPage).toContain("CONTENT_FILTER_WHITELIST_FIELDS");
    expect(whitelistPage).not.toContain("CONTENT_FILTER_WORD_FIELDS");
    // 两页都必须读-改-写，而不是把自己那份整段直接写回去。
    for (const source of [words, whitelistPage]) {
      expect(source).toContain("mergeContentFilterPayload");
      expect(source).toContain("getContentFilterRuntimeConfig()");
    }
  });

  it("白名单 tab 是指向独立页的链接，页面文件真实存在", () => {
    const tabs = readSettings("ContentFilterTabs.vue");

    expect(tabs).toContain('to="/content/keywords/whitelist"');
    expect(
      existsSync(join(projectDir, "app", "pages", "content", "keywords", "index.vue")),
    ).toBe(true);
    expect(
      existsSync(join(projectDir, "app", "pages", "content", "keywords", "whitelist.vue")),
    ).toBe(true);
  });
});

/**
 * 6 万条词表撑破后台页（2026-11-22 线上事故）：段响应 ~3.8MB JSON 未压缩，
 * 默认 15s 超时 → 页面报「配置加载失败」；即便取回来，6 万个 <button> 也会把
 * 浏览器卡住。这两条断言钉住修法：放宽超时 + 只渲染一窗。
 */
describe("内容过滤页的规模防护", () => {
  it("取/存这一段都用放宽后的超时（不是默认 15s）", () => {
    const service = readFileSync(join(process.cwd(), "app", "services", "admin.ts"), "utf8");

    expect(service).toContain("CONTENT_FILTER_TIMEOUT_MS");
    expect(service).toContain("getRuntimeSectionWithTimeout");
    expect(service).toContain("updateRuntimeSectionWithTimeout");
  });

  it("词云只渲染一窗，并给出「还有 N 个」的出口", () => {
    const source = readWordsPage();

    expect(source).toContain("RENDER_WINDOW");
    expect(source).toContain("visibleWords");
    expect(source).toContain("hiddenWordCount");
    expect(source).toContain("content-filter-show-more");
    // 模板不能还按全量渲染：那正是卡死浏览器的原因。
    expect(source).toContain('v-for="entry in visibleWords"');
    expect(source).not.toContain('v-for="entry in filteredWords"');
  });
});
