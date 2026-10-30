// Tailwind 配置探针：把项目自己的 tailwind 配置编译一遍，断言生成的 CSS。
//
// 为什么不用断言"配置文件里有某个字段"了事：那个断言在配置没被 Nuxt 加载、
// 或者 Tailwind 根本编译不过的时候照样是绿的（这门禁得能红）。这里走的是
// Tailwind 真正的 postcss 插件管线，产物就是构建期的产物：
//   - 语义色/排版 token 类名是否真的生成了 CSS 声明；
//   - `dark:` 工具类挂在 `.dark` 祖先上（class 策略），而不是 `prefers-color-scheme`
//     —— 后者正是"站内开关管不住 dark: 工具类"的历史 bug。
//
// 依赖解析：两端都没有把 tailwindcss / postcss 声明成直接依赖（它们来自
// @nuxtjs/tailwindcss），pnpm isolated 布局下从项目根 require 是拿不到的。
// 所以从 @nuxtjs/tailwindcss 包的真实路径出发建一个 require —— 它的依赖
// （tailwindcss / postcss）就在同一个 .pnpm 目录里。
import { realpathSync } from "node:fs";
import { createRequire } from "node:module";
import { join } from "node:path";

/** 解析项目可用的 postcss / tailwindcss / loadConfig。 */
export function loadProjectTailwind(projectDir) {
  const moduleDir = realpathSync(join(projectDir, "node_modules", "@nuxtjs", "tailwindcss"));
  const requireFromModule = createRequire(join(moduleDir, "package.json"));
  const loadConfigModule = requireFromModule("tailwindcss/loadConfig.js");
  return {
    postcss: requireFromModule("postcss"),
    tailwindcss: requireFromModule("tailwindcss"),
    loadConfig: loadConfigModule.default ?? loadConfigModule,
  };
}

/**
 * 用项目自己的 tailwind.config 编译一段探针 HTML，返回生成的 CSS。
 * @param {{ projectDir: string, html: string }} input
 * @returns {Promise<string>}
 */
export async function compileTailwindProbe({ projectDir, html }) {
  const { postcss, tailwindcss, loadConfig } = loadProjectTailwind(projectDir);
  const userConfig = loadConfig(join(projectDir, "tailwind.config.ts"));
  // content 换成探针 HTML：既要保证用真实配置，又要保证探针里写的类名一定会被扫到。
  const config = { ...userConfig, content: [{ raw: html, extension: "html" }] };
  const result = await postcss([tailwindcss(config)]).process("@tailwind utilities;", {
    from: undefined,
  });
  return result.css;
}
