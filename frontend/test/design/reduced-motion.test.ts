import { readFileSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

// 目录遍历收进 helpers。注意这里扫的是**两个端**的 app，且要跳过 node_modules
// 之类的目录（admin 的 app 下没有这些，但 frontend/app 链路上保留原跳过清单不动）。
import { collectFiles, extensionFilter } from "../helpers/designFiles";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");
const appRoots = [join(projectDir, "app"), join(repoRoot, "frontend_admin/app")];

const sources = appRoots.flatMap((root) =>
  collectFiles(root, extensionFilter("vue", "css"))
    .map((file) => ({
      file: relative(repoRoot, file).replace(/\\/gu, "/"),
      text: readFileSync(file, "utf8"),
    }))
    // 注释里的说明不参与审计（只认真实声明）。
    .map((entry) => ({ ...entry, code: entry.text.replace(/\/\*[\s\S]*?\*\//gu, "") })),
);

describe("reduced-motion 全站审计（无未防护的循环动画）", () => {
  it("任何带 animation / @keyframes 的文件都必须有 prefers-reduced-motion 兜底", () => {
    const animated = sources.filter(({ code }) => /@keyframes|\banimation(?:-name)?\s*:/u.test(code));

    // 审计确实覆盖到了动画文件（不是"没动画所以全绿"）。
    expect(animated.length).toBeGreaterThan(0);

    const unprotected = animated
      .filter(({ text }) => !text.includes("prefers-reduced-motion"))
      .map(({ file }) => file);

    expect(unprotected).toEqual([]);
  });

  it("无限循环动画在「减少动态」下都必须被关掉（不能只是换时长）", () => {
    const infinite = sources.filter(({ code }) => /infinite/u.test(code));

    expect(infinite.length).toBeGreaterThan(0);
    for (const { file, text } of infinite) {
      expect(text, file).toContain("animation: none");
    }
  });

  it("没有滚动驱动动画 / 滚动编排残留", () => {
    for (const { file, text } of sources) {
      expect(text, file).not.toMatch(/animation-timeline|scroll-timeline|scroll-driven/u);
    }
  });
});
