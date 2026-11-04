import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { findColorLiterals } from "../../../scripts/check-design-tokens.mjs";

/**
 * 阅读页左栏的接线（#220 / 决定清单 A9–A11）。
 *
 * 阅读页是 700 行的 SSR 路由页，整页 mount 会拖进整套取数 composable；这里沿用
 * #213 的先例：只断言页面源码里的接线（判定在可单测的 composable/纯函数里）。
 *
 * 守住的边界：
 *  - 树只在客户端取数（不在 setup / SSR 里 await 或注入请求，ADR-0019）；
 *  - 有归属树才出现左栏，且三栏布局在 1024px 以上成立；
 *  - 右栏标题大纲与阅读进度保持原样，不被左栏替换。
 */

const PAGE_FILE = "app/pages/article/[id].vue";

function pageSource(): string {
  return readFileSync(join(process.cwd(), PAGE_FILE), "utf8");
}

/** 截出正文布局（`.article-layout` 到收尾 `</div>` 之前），三栏断言都在这块里做。 */
function layoutSource(source: string): string {
  const start = source.indexOf('<div class="article-layout"');
  expect(start, "页面里找不到正文布局 .article-layout").toBeGreaterThan(-1);
  const end = source.indexOf("</aside>\n      </div>", start);
  expect(end, "找不到正文布局的收尾").toBeGreaterThan(start);
  return source.slice(start, end);
}

describe("文章页：左栏目录树的接线", () => {
  it("来源取自路由 ?from=，交给可单测的取数 composable（不在页面里手写请求）", () => {
    const source = pageSource();

    expect(source).toContain('import ArticleOutlineAside from "~/components/article/ArticleOutlineAside.vue";');
    expect(source).toContain('import { useArticleOutlineContext } from "~/composables/useArticleOutlineContext";');
    expect(source).toMatch(/route\.query\.from/u);
    expect(source).toMatch(
      /useArticleOutlineContext\(\{\s*articleId(?:\s*:\s*articleId)?,\s*from(?:\s*:\s*outlineFrom)?\s*\}\)/u,
    );
    // 页面上没有第二份取数调用：请求只在 service/composable 里发。
    expect(source).not.toMatch(/outline-context/u);
  });

  it("树只在客户端取数：setup 不 await 上下文请求，也不在 SSR 阶段注入取数", () => {
    const source = pageSource();

    expect(source).not.toMatch(/await\s+\w*outline/i);
    expect(source).not.toMatch(/useAsyncData[\s\S]{0,80}outline/i);
    // SSR 边界与匿名缓存策略的守卫在 test/gates/rendering-boundary.test.ts 与
    // ssr-anonymous-cache.test.ts；这里只钉住"页面不额外取数"。
  });

  it("有归属树（pending 或命中）时才渲染目录抽屉，无归属不出现", () => {
    const source = pageSource();

    // 抽屉不再住进 .article-layout：它挪到页面末尾（与评分弹窗同级），是浮层。
    const layout = layoutSource(source);
    expect(layout).not.toContain("<ArticleOutlineAside");
    expect(source).toMatch(/<ArticleOutlineAside\s+v-if="outlineColumnVisible"/u);
    expect(source).toContain(":current-article-id=\"articleId\"");
    // 抽屉在布局之后：`</div>`（.article-layout 收尾）之后才出现。
    expect(source.indexOf("<ArticleOutlineAside")).toBeGreaterThan(source.indexOf('<div class="article-layout"'));
  });

  it("正文布局恒定两列：不再有 third-column 修饰类（目录不挤正文）", () => {
    const source = pageSource();
    const css = readFileSync(join(process.cwd(), "app/assets/css/tailwind.css"), "utf8");

    expect(source).toContain('<div class="article-layout">');
    expect(source).not.toContain("article-layout--with-outline");
    expect(css).not.toContain(".article-layout--with-outline");
  });

  it("切页骨架形态恒定：不再按归属树切换骨架", () => {
    const source = pageSource();

    expect(source).toMatch(/<ArticlePageSkeleton\s+v-if="articleSkeleton"\s*\/>/u);
    expect(source).not.toContain(":with-outline");
  });

  it("右栏标题大纲与阅读进度保持原样（不被左栏替换）", () => {
    const source = pageSource();

    expect(source).toContain("useReadingProgress(");
    expect(layoutSource(source)).toContain("<ArticleTocAnchor");
    expect(source).toContain(":progress-percent=\"progressPercent\"");
    expect(source).toMatch(/<ArticleTocAnchor\s+:headings="articleHeadings"/u);
  });

  it("左栏样式留在组件里：页面仍不引入 <style> 块，也没有裸色值", () => {
    const source = pageSource();

    expect(source).not.toContain("<style");
    expect(findColorLiterals(source)).toEqual([]);
  });
});
