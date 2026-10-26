import { fileURLToPath } from "node:url";
import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vitest/config";

const appRoot = fileURLToPath(new URL("./app", import.meta.url));

export default defineConfig({
  plugins: [vue()],
  define: {
    "import.meta.client": "true",
    "import.meta.server": "false",
  },
  resolve: {
    alias: {
      "~": appRoot,
      "@": appRoot,
    },
  },
  test: {
    environment: "happy-dom",
    globals: false,
    include: ["test/**/*.{test,spec}.ts"],
    // 曾经是 true：那时一个用例都没有，为了让空跑不失败。
    // 现在第一批用例已落地（角色/状态、文章文案、格式化），改为 false ——
    // 否则"测试文件被误删"和"全部通过"在门禁里长得一模一样。
    passWithNoTests: false,
    restoreMocks: true,
    clearMocks: true,
    coverage: {
      provider: "v8",
      reporter: ["text"],
      // 只统计**纯逻辑模块**。
      //
      // 为什么不覆盖整个 app：页面与组件需要 mock 一整套 naive-ui 交互，
      // 投入产出比低。把门槛限定在 utils 上，含义是"已经测过的东西不许退化"，
      // 而不是"用一个虚高的数字粉饰整站 0 覆盖"。
      //
      // 说清楚现状：仓库里**没有 e2e**（无 playwright/cypress，CI 也没有 e2e job），
      // 所以页面层的自动化保障目前是 0 —— 回归靠手测。之前这里写着"读者站那边
      // 这类展示组件也是靠 e2e 覆盖的"，那句话与事实不符，已更正。
      include: ["app/utils/**/*.ts"],
      // 2026-10-26 首次实测 100/100/100/100（14 个用例）。
      // 门槛按实测值定，之后新增工具函数必须同时补用例。
      thresholds: {
        statements: 95,
        branches: 90,
        functions: 95,
        lines: 95,
      },
    },
  },
});
