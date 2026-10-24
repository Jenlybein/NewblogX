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
