import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 内容过滤「白名单」两层（ADR-0032 补丁四）。
 *
 * 后台没有组件测试基线（naive-ui 交互要 mock 一整套），所以按源码断言 ——
 * 与 settings-wording.test.ts 同一套做法。这里钉三件在评审里被点出来的事：
 *
 * 1. 短语区必须绑 `whitelist_phrases`（只改文案不接字段等于没做）；
 * 2. Tab 计数必须用后端下发的 `whitelist_stats.total`（前端自己把两个数组相加
 *    会出现"页面 50、体检 49"）；
 * 3. 边界说明必须在（短语只覆盖已知连续写法、长短语会连带豁免其中其它敏感词）——
 *    上一版用"越长越安全"顶替了精确说明，运营会以为它能自动覆盖变体。
 */
const settingsDir = join(process.cwd(), "app", "components", "settings");

function readContentFilter(): string {
  return readFileSync(join(settingsDir, "SettingsContentFilter.vue"), "utf8");
}

describe("白名单两层（ADR-0032 补丁四）", () => {
  it("短语区绑定 whitelist_phrases，并有自己的保存按钮", () => {
    const source = readContentFilter();

    expect(source).toContain("config.whitelist_phrases");
    expect(source).toContain("保存短语白名单");
  });

  it("Tab 计数用后端口径，不自己把两个数组相加", () => {
    const source = readContentFilter();

    expect(source).toContain("whitelist_stats");
    expect(source).not.toContain("whitelist.length + whitelistPhrases.length");
  });

  it("写清短语的边界，不用「越长越安全」笼统带过", () => {
    const source = readContentFilter();

    expect(source).toContain("完整包住");
    expect(source).toContain("已知的连续写法");
    expect(source).toContain("连带");
    expect(source).not.toContain("越长越安全");
  });
});
