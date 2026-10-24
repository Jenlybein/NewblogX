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
    // 管理后台当前没有任何测试用例。
    // 保留测试入口而不是"不配"，是为了让"加第一个测试"没有门槛——
    // 一个没有 test 脚本的 app，实际上永远不会有人开始写测试。
    include: ["test/**/*.{test,spec}.ts"],
    passWithNoTests: true,
    restoreMocks: true,
    clearMocks: true,
  },
});
