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
    },
  },
});
