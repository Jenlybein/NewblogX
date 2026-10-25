// ESLint flat config（管理后台）
//
// 与读者站完全独立：不 import 任何跨目录配置，各 app 自洽。
import withNuxt from "./.nuxt/eslint.config.mjs";

export default withNuxt(
  {
    rules: {
      "no-debugger": "error",
      "no-console": ["warn", { allow: ["warn", "error"] }],
      // 本项目是 Nuxt 4 / Vue 3，模板允许多根节点与 <slot/> 作为根。
      // 这条规则是 Vue 2 遗留，开着只会误报（layouts 就是只放一个 <slot/>）。
      "vue/no-multiple-template-root": "off",
    },
  },
  {
    // S5.6 的验收口径（单个文件 ≤800 行）落到门禁上，与读者站同一把尺子。
    // 为什么是 error：warn 不会让 `eslint .` 失败，等于没有这道门。
    // 现状：最大的 `app/pages/media/index.vue` 328 行，余量充足。
    files: ["app/**/*.ts", "app/**/*.vue"],
    ignores: [
      // openapi 生成的类型声明（改不了也不该手改）
      "app/types/openapi.d.ts",
    ],
    rules: {
      "max-lines": ["error", { max: 800 }],
    },
  },
  {
    ignores: [
      ".nuxt/**",
      ".output/**",
      "dist/**",
      "coverage/**",
      "node_modules/**",
    ],
  },
);
