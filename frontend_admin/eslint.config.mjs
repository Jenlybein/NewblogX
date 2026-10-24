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
    ignores: [
      ".nuxt/**",
      ".output/**",
      "dist/**",
      "coverage/**",
      "node_modules/**",
    ],
  },
);
