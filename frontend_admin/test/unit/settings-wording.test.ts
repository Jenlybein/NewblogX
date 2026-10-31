import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 后台「站点设置」页各 Tab 的保存文案（#39）。
 *
 * 为什么值得一条用例：AI 配置一直是**独立段、独立接口、独立 Tab**，但 AI Tab 的按钮和
 * 成功提示曾写成「保存站点级设置」/「站点级设置已保存」—— 这句话会被读成"保存 site 段的
 * 配置"，暗示 AI 与站点基础信息耦合。兄弟 Tab 都是「保存邮件配置」「保存日志配置」，
 * 所以这里把句式钉住：按钮是「保存 <段名> 配置」，提示是「<段名> 配置已保存」。
 *
 * 后台前端没有组件测试基线（naive-ui 交互要 mock 一整套），这些文案目前只存在于 .vue
 * 模板里，因此按源码断言 —— 与 `legacy-colors.test.ts` 同一套做法。
 *
 * vitest 以 project root（frontend_admin/）为 cwd。
 */
const projectDir = process.cwd();
const settingsDir = join(projectDir, "app", "components", "settings");

function readComponent(name: string): string {
  return readFileSync(join(settingsDir, name), "utf8");
}

describe("AI Tab 的保存文案按段称呼（#39）", () => {
  it("按钮与成功提示都写「AI 助手配置」", () => {
    const source = readComponent("SettingsAi.vue");

    expect(source).toContain("保存 AI 助手配置");
    expect(source).toContain("AI 助手配置已保存");
  });

  it("不再出现会被读成「保存 site 段」的「站点级设置」", () => {
    const source = readComponent("SettingsAi.vue");

    expect(source).not.toContain("站点级设置");
  });
});

describe("兄弟 Tab 的文案句式（改动不能只落在 AI Tab 上）", () => {
  it("邮件 / 日志 Tab 仍是「保存 <段名> 配置」", () => {
    expect(readComponent("SettingsMail.vue")).toContain("保存邮件配置");
    expect(readComponent("SettingsLog.vue")).toContain("保存日志配置");
  });
});
