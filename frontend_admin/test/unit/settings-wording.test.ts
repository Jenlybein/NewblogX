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

  it("新增的装饰 Tab 也照同一句式（票 #40）", () => {
    const source = readComponent("SettingsDeco.vue");
    expect(source).toContain("保存装饰配置");
    expect(source).toContain("装饰配置已保存");
  });
});

describe("装饰 Tab 的「恢复默认」（票 #42）", () => {
  it("按钮存在，且重置只写本地表单、不自动保存到服务器", () => {
    const source = readComponent("SettingsDeco.vue");

    expect(source).toContain("恢复默认");
    // 恢复默认的目标值由工具层统一提供（九个默认值），组件不自己写数字。
    expect(source).toContain("defaultDecoConfig");
    // 反例守卫：重置处理函数里不能出现任何写接口调用。切出 `resetDefaults`
    // 的函数体，断言它没碰 updateDecoRuntimeConfig / save( )。
    const resetBody = source.match(/function resetDefaults\(\) \{([\s\S]*?)\}/u)?.[1] ?? "";
    expect(resetBody.length).toBeGreaterThan(0);
    expect(resetBody).not.toContain("updateDecoRuntimeConfig");
    expect(resetBody).not.toContain("save(");
  });

  it("数值输入行由元数据驱动，不是手写固定五行", () => {
    const source = readComponent("SettingsDeco.vue");

    expect(source).toContain("v-for=\"param in numberParams\"");
    // 行数由 DECO_NUMBER_PARAMS 决定；模板里不能出现逐字段的数值绑定，
    // 否则参数清单与表单渲染就会有两份真相（加参数时只改元数据不再成立）。
    expect(source).toContain("const numberParams = DECO_NUMBER_PARAMS");
    expect(source).not.toMatch(
      /config\.charm\.(logo_size|rope_length|segments|line_width|gravity|damping|idle_sway_hz|idle_sway_amp)/u,
    );
  });
});
