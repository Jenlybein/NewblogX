import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { resolveMarkdownTheme } from "~/utils/markdownTheme";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");

const chatSurfaces = ["app/components/inbox/InboxChatPanel.vue", "app/components/jelly/JellyAiPanel.vue"].map((file) => ({
  file,
  text: readFileSync(join(projectDir, file), "utf8"),
}));

/** 断言只看真实代码：说明"不再引 github-markdown"的注释不该被当成引用。 */
function stripComments(text: string): string {
  return text
    .replace(/<!--[\s\S]*?-->/gu, "")
    .replace(/\/\*[\s\S]*?\*\//gu, "")
    .replace(/^[ \t]*\/\/.*$/gmu, "");
}

describe("聊天面 markdown 主题收口（暗色下不许白块）", () => {
  it("自有主题在暗色下走中性暗面，而不是白色", () => {
    const light = resolveMarkdownTheme(tokensCss, "light");
    const dark = resolveMarkdownTheme(tokensCss, "dark");

    // 明暗两套取值确实翻转：正文由中性墨变浅石墨，纸面由奶油白变中性暗面。
    expect(light.body).toBe("#1c1b19");
    expect(dark.body).toBe("#f5f5f7");
    expect(light.paper).toBe("#fdfbf6");
    expect(dark.paper).toBe("#1d1d1f");
    expect(light.paperRaised).not.toBe(dark.paperRaised);

    // 暗色下任何"纸面/正文底"都不能是纯白（否则就是聊天面白块）。
    for (const role of ["paper", "paperRaised", "subtle", "page"] as const) {
      expect(dark[role].toLowerCase()).not.toMatch(/^#f{5,6}$|^#fff(fff)?$/u);
    }
  });

  it("私信 / AI 面板不再引用 github-markdown-css 或 highlight 外部主题", () => {
    for (const { file, text } of chatSurfaces) {
      const code = stripComments(text);

      expect(code, file).not.toContain("github-markdown");
      expect(code, file).not.toContain("highlight.js/styles");
      // 不再显式传外部主题 href —— 主题由 shadow root 内置。
      expect(code, file).not.toMatch(/:theme-href=/u);
    }
  });

  it("两个聊天面仍保留 katex 基线样式（公式排版依赖它）", () => {
    for (const { file, text } of chatSurfaces) {
      expect(text, file).toContain("katex/dist/katex.min.css?url");
      expect(text, file).toContain("extra-style-hrefs");
    }
  });
});
