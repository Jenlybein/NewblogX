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
      "#imports": fileURLToPath(new URL("./test/mocks/nuxt-imports.ts", import.meta.url)),
      "#markdown-it": fileURLToPath(new URL("./node_modules/markdown-it/index.mjs", import.meta.url)),
      "#markdown-it-ins": fileURLToPath(new URL("./node_modules/markdown-it-ins/index.mjs", import.meta.url)),
    },
  },
  test: {
    environment: "happy-dom",
    globals: false,
    setupFiles: ["./test/setup.ts"],
    include: ["test/**/*.{test,spec}.ts"],
    restoreMocks: true,
    clearMocks: true,
    coverage: {
      provider: "v8",
      reporter: ["text", "html"],
      reportsDirectory: "./coverage",
      exclude: [
        ".nuxt/**",
        ".output/**",
        "coverage/**",
        "node_modules/**",
        "test/**",
        "app/types/**",
      ],
      // 覆盖率门槛存在的意义：防止有人把测过的代码删掉、或把难测的分支
      // 挪到没测试的地方，导致覆盖率"静默下降"而没人发现。
      //
      // 演进：
      //   2026-10-24 实测 62.47 / 50.96 / 67.14 / 62.62 → 门槛 60 / 50 / 65 / 60
      //   2026-10-26 **下调**到 57 / 47 / 63 / 57 —— 一次有意的例外，原因见下
      //   2026-10-26 补完两个写作页 composable 后**上调**到 60 / 50 / 66 / 60
      //
      // 为什么曾下调：M6 把写作页 2045 行拆成 361 行页面 + 约 2400 行模块
      // （6 个 composable + 4 个组件 + 2 张常量表）。拆分前那一整块在分母里且
      // 部分被间接触及；拆分后新增模块独立成文件、几乎全是 0 覆盖，于是整体
      // 掉 3 个百分点。**这不是测试被删，是分母变大。**
      //
      // 为什么现在能上调：useStudioArticle（26 用例）与 useStudioAiAssist（23 用例）
      // 这两块约 560 行曾经零覆盖，是门槛卡在 57 的直接原因。补完后实测
      // 63.02 / 51.83 / 67.85 / 62.89，用例数 125 → 174。
      //
      // 剩下的待补清单（补完继续上调）：
      //   StudioWriteHeader / StudioWritePublishDialog —— 展示组件，靠 e2e 更划算
      //
      // 反例（不要这么做）：把 components/pages 从统计里排除来凑数字 ——
      // 实测排除后反而更低（52%），因为那两个目录原本的覆盖率高于 composables。
      thresholds: {
        statements: 60,
        branches: 50,
        functions: 66,
        lines: 60,
      },
    },
  },
});
