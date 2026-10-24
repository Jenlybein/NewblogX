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
      // 数值 = 2026-10-24 实测值向下取整（stmt 62.47 / branch 50.96 /
      // func 67.14 / line 62.62），作为只升不降的基线。
      // 后续每补一批测试就上调，永不回调。
      thresholds: {
        statements: 60,
        branches: 50,
        functions: 65,
        lines: 60,
      },
    },
  },
});
