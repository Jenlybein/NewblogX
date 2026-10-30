import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");
const appRoots = [join(projectDir, "app"), join(repoRoot, "frontend_admin/app")];

function walk(dir: string, files: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      if (["node_modules", ".nuxt", ".output", "coverage"].includes(name)) continue;
      walk(full, files);
      continue;
    }
    if (full.endsWith(".vue") || full.endsWith(".css")) {
      files.push(full);
    }
  }
  return files;
}

const sources = appRoots.flatMap((root) =>
  walk(root)
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
