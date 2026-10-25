// ESLint flat config（读者站）
//
// 目标不是"零告警"，而是让 lint 成为 CI 里真的会失败的一道门。
// 规则集沿用 @nuxt/eslint 的预设（含 Vue + TS 推荐规则），
// 这里只补几条与本项目直接相关的约束。
import withNuxt from "./.nuxt/eslint.config.mjs";

export default withNuxt(
  {
    rules: {
      // 禁止把 debugger 提交到仓库（真会导致线上断点）
      "no-debugger": "error",
      // 生产代码禁止裸 console.log（warn/error 允许，它们通常是刻意的）
      "no-console": ["warn", { allow: ["warn", "error"] }],
      // Vue SFC 里自闭合写法（<img />）是惯用风格，且写法统一比"必须去掉斜杠"
      // 更重要。这条规则开着只会让 --fix 把几十个文件各翻一行，制造噪音 diff。
      "vue/html-self-closing": "off",
    },
  },
  {
    // S5.6 的验收口径（单个文件 ≤800 行）落到门禁上。
    //
    // 为什么是 error 而不是 warn：warn 不会让 `eslint .` 失败，等于这道门不存在
    // —— 写作页正是在"没人拦"的情况下长到 2,045 行的。
    //
    // 为什么用原始行数（空行、注释照算）：与后端 R6「单文件不得超过 800 行」
    // 用同一把尺子，读前后端文档时不用在两套口径之间换算。
    //
    // 现状：最大的 `app/pages/article/[id].vue` 791 行，只剩 9 行余量。
    // 它下次再往里加东西就会红 —— 这是有意的。红的时候正确的做法是把 <style>
    // 外移（写作页就是这么从 2,045 降到 361 的），而不是把上限调高。
    files: ["app/**/*.ts", "app/**/*.vue"],
    ignores: [
      // openapi 生成的类型声明（6,000+ 行，改不了也不该手改）
      "app/types/openapi.d.ts",
    ],
    rules: {
      "max-lines": ["error", { max: 800 }],
    },
  },
  {
    // 这些目录是生成产物或第三方代码，不参与 lint
    ignores: [
      ".nuxt/**",
      ".output/**",
      "dist/**",
      "coverage/**",
      "node_modules/**",
    ],
  },
);
