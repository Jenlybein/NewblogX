import { readFileSync, readdirSync } from "node:fs";
import { join, relative, sep } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 无障碍地标骨架（#434，spec #423 第 3 批第 45 条）。
 *
 * 这里钉的是**页面结构的可观察契约**，不是某个组件的实现：
 *   1. 布局层（default / studio）各正好一个 `<main>` 地标；
 *   2. 布局已经给出 `<main>` 的页面不再自带第二个 `<main>` —— 否则读屏的
 *      "跳到正文" 会落到内层那个假地标上，`main` 也会被报成两个；
 *   3. skip link 的 `#main-content` 全站只有一个命中点（布局层那个 main）。
 *
 * 详情页（collections/[id]、projects/[id]）与列表页共用 default 布局，历史上
 * 也各写了一个 `<main id="main-content">`；只修列表页会让"全站只有布局层一个
 * main"不成立，所以这道门禁按**全站**口径钉。
 */

const projectDir = process.cwd();

function read(path: string): string {
  return readFileSync(join(projectDir, path), "utf8");
}

/** 递归列出 app 下指定扩展名的文件，路径统一成正斜杠（Windows / POSIX 同口径）。 */
function collectFiles(dir: string, extension: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(join(projectDir, dir), { withFileTypes: true })) {
    const child = join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...collectFiles(child, extension));
      continue;
    }
    if (!entry.name.endsWith(extension)) continue;
    out.push(relative(projectDir, join(projectDir, child)).split(sep).join("/"));
  }
  return out;
}

const appVueFiles = collectFiles("app", ".vue");
const pageFiles = appVueFiles.filter((file) => file.startsWith("app/pages/"));

/** 某个文件里出现了几个 `<main` 开标签。 */
function mainTagsIn(source: string): number {
  return (source.match(/<main\b/gu) ?? []).length;
}

function occurrencesOf(files: string[], pattern: RegExp): string[] {
  return files.filter((file) => pattern.test(read(file)));
}

describe("单一 <main> 地标", () => {
  it("default 布局正好一个 main，并带上 skip link 的目标 id", () => {
    const layout = read("app/layouts/default.vue");

    expect(mainTagsIn(layout)).toBe(1);
    expect(layout).toContain('<main id="main-content"');
  });

  it("studio 布局也正好一个 main（它下面没有页面自带 main）", () => {
    expect(mainTagsIn(read("app/layouts/studio.vue"))).toBe(1);
  });

  it("只有 write 页面自带 main —— 因为 write 布局刻意不渲染站点页头/地标", () => {
    const pagesWithMain = occurrencesOf(pageFiles, /<main\b/u);

    expect(pagesWithMain).toEqual(["app/pages/studio/write.vue"]);
    expect(mainTagsIn(read("app/layouts/write.vue"))).toBe(0);
  });

  it("合集 / 项目列表页与详情页不再自带 main：内层改成语义容器", () => {
    for (const file of [
      "app/pages/collections/index.vue",
      "app/pages/collections/[id].vue",
      "app/pages/projects/index.vue",
      "app/pages/projects/[id].vue",
    ]) {
      const source = read(file);

      expect(mainTagsIn(source), `${file} 仍带 <main>`).toBe(0);
      expect(source, `${file} 应改用 <section> 容器`).toMatch(/<section\b/u);
    }
  });
});

describe("skip link 唯一命中", () => {
  it("全站只有一个 id=main-content，且只在 default 布局上", () => {
    const filesWithTarget = occurrencesOf(appVueFiles, /id="main-content"/u);

    expect(filesWithTarget).toEqual(["app/layouts/default.vue"]);
    expect((read("app/layouts/default.vue").match(/id="main-content"/gu) ?? []).length).toBe(1);
  });

  it("页头正好一个 skip link，指向那个唯一的正文地标", () => {
    const header = read("app/components/layout/AppHeader.vue");
    const links = header.match(/href="#main-content"/gu) ?? [];

    expect(links).toHaveLength(1);
    expect(header).toContain('data-chrome="skip-link"');
  });
});
