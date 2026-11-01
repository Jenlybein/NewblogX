import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { resolveSearchEmptyState, type SearchEmptyContext } from "~/utils/searchState";
import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 读者站解释性文案清理（#126 / 父票 #95 的「文案」一项）。
 *
 * 为什么用源码扫描而不是挂载页面：搜索页、首页、文章页都依赖 Nuxt 运行时（useAsyncData /
 * useRoute / 自动导入），本仓库没有页面级挂载测试的先例；父票 #95 的 Testing Decisions 也
 * 明确认可"以这些字符串不再出现在页面渲染结果里为断言"（`test/gates/` 已有读源码断言的先例）。
 * 所以这里只锁两件事：
 *   1. 清单里的解释性文案**从代码里消失**（不是被注释/隐藏——扫描前先去注释）；
 *   2. 反向：真正的空态 / 错误态 / 加载态引导**仍在**，分页的事实信息（页码/条数）与按钮组的
 *      对齐**仍在**。
 *
 * 判据写成纯函数 + 人造违规用例（canary）：只断言现状的扫描是恒绿的，判据被改坏时没人会发现。
 * 文案清单的唯一来源就是本文件（删掉的句子没有运行时消费者，不值得为它再造一个模块）。
 */

/** 五处清单文案的可识别片段（逐条对应 #126 的清单，含同义孪生）。 */
const REMOVED_READER_COPY = [
  "关键词搜索标题、摘要与正文",
  "也可以按标签筛选，或切到智能搜索",
  "未刷新前，已访问页会直接复用缓存",
  "已访问页会复用缓存，翻页不重复请求旧页",
  "已加载 {{ commentPagesLoaded }} 页",
  "可以搜标题、摘要或正文",
  "也可以只选一个标签，或切到智能搜索",
] as const;

/** 判定：源码里还残留哪几句被删文案（去注释后再扫，避免"注释里提到就当没删"）。 */
function findRemovedCopy(source: string): string[] {
  const stripped = stripComments(source);
  return REMOVED_READER_COPY.filter((phrase) => stripped.includes(phrase));
}

const projectDir = process.cwd();
const readerFiles = {
  searchPage: join(projectDir, "app/pages/search.vue"),
  homePage: join(projectDir, "app/pages/index.vue"),
  articlePage: join(projectDir, "app/pages/article/[id].vue"),
  searchState: join(projectDir, "app/utils/searchState.ts"),
  tailwindCss: join(projectDir, "app/assets/css/tailwind.css"),
};

function read(path: string): string {
  return readFileSync(path, "utf8");
}

/** 取 `marker` 所在的那个唯一开标签（`marker` 必须落在标签内部，如 class / data-role）。 */
function openingTagFor(source: string, marker: string): string {
  const markerIndex = source.indexOf(marker);
  expect(markerIndex, `找不到 ${marker}`).toBeGreaterThan(-1);
  const start = source.lastIndexOf("<div", markerIndex);
  const end = source.indexOf(">", markerIndex);
  expect(start, `${marker} 不在 <div> 开标签内`).toBeGreaterThan(-1);
  expect(end).toBeGreaterThan(markerIndex);
  return source.slice(start, end + 1);
}

describe("门禁不能恒绿：判据必须能抓到残留文案", () => {
  it("人造源码里逐条塞回清单文案，每一条都会被扫出来", () => {
    for (const phrase of REMOVED_READER_COPY) {
      expect(findRemovedCopy(`<p>${phrase}</p>`)).toEqual([phrase]);
    }
  });

  it("全部残留时一次报全，干净的源码返回空数组", () => {
    const polluted = REMOVED_READER_COPY.map((phrase) => `<span>${phrase}</span>`).join("\n");
    expect(findRemovedCopy(polluted)).toEqual([...REMOVED_READER_COPY]);
    expect(findRemovedCopy("<div class=\"page-stack\" />")).toEqual([]);
  });

  it("提示文案（空态 / 错误态的引导）不在清单里，塞进去也不会被误报", () => {
    const guidance = [
      "正在读取匹配结果，请稍候。",
      "先重试本页；仍然失败就清空条件换个关键词。",
      "关键词越具体，结果越准。",
      "搜索是在公开文章范围内进行的，未发布内容不会出现。",
    ].join("\n");

    expect(findRemovedCopy(guidance)).toEqual([]);
  });
});

describe("搜索页：顶部说明段与分页条说明消失", () => {
  const searchPage = () => read(readerFiles.searchPage);

  it("顶部那句「关键词搜索标题、摘要与正文…」不再出现在页面里", () => {
    expect(findRemovedCopy(searchPage())).toEqual([]);
  });

  it("顶部说明段连标签一起删：装饰性 eyebrow「Search」标签不残留", () => {
    expect(searchPage()).not.toContain('class="eyebrow">Search');
  });

  it("页面标题仍保留：删说明不等于把标题区一起删空", () => {
    expect(searchPage()).toContain("<h1");
    expect(searchPage()).toContain("搜索文章");
  });

  it("分页条里的「复用缓存」说明与首屏标签都不留空段", () => {
    const template = searchPage();
    const pagerTag = openingTagFor(template, 'v-if="showPager"');

    expect(template).not.toContain("复用缓存");
    // 说明段删除后分页行只剩按钮组：容器不再是 justify-between，按钮靠右对齐、间距仍在。
    expect(pagerTag).toContain("justify-end");
    expect(pagerTag).not.toContain("justify-between");
    expect(pagerTag).toContain("gap-3");
    expect(template.slice(template.indexOf('v-if="showPager"'), template.indexOf("上一页")).includes("blueprint-actions")).toBe(true);
  });
});

describe("同义孪生：首页与文章页分页条的同类说明一并消失", () => {
  it("首页分页条不再解释缓存复用，按钮组对齐不被打乱", () => {
    const home = read(readerFiles.homePage);
    const pagerTag = openingTagFor(home, 'data-role="feed-pager"');

    expect(pagerTag).toContain("justify-end");
    expect(pagerTag).not.toContain("justify-between");
    expect(pagerTag).toContain("gap-3");
    expect(home.slice(home.indexOf('data-role="feed-pager"'), home.indexOf("上一页")).includes("blueprint-actions")).toBe(true);
    expect(findRemovedCopy(home)).toEqual([]);
  });

  it("文章页评论分页条只删解释性措辞，页码/条数这类事实信息保留", () => {
    const article = read(readerFiles.articlePage);

    expect(findRemovedCopy(article)).toEqual([]);
    expect(article).not.toContain("已加载");
    // 事实信息留着：读者仍知道自己翻到第几页。
    expect(article).toMatch(/评论第\s*\{\{\s*commentPage\s*\}\}\s*页/u);
  });

  it("文章页分页行仍带左侧事实信息：justify-between 与间距类照旧，按钮组不被挤歪", () => {
    const css = read(readerFiles.tailwindCss);
    const rowBlock = blockAfter(css, ".comment-panel__pagination {");
    const actionsBlock = blockAfter(css, ".comment-panel__pagination-actions {");

    // 文章页保留「评论第 N 页，每页 7 条」事实信息，左侧仍有元素，所以两端对齐不变。
    expect(rowBlock).toContain("justify-between");
    expect(rowBlock).toContain("flex");
    expect(rowBlock).toContain("gap-3");
    expect(actionsBlock).toContain("flex");
    expect(actionsBlock).toContain("gap-2");
  });
});

describe("搜索页空态：与顶部说明重复的那句描述也删", () => {
  const baseContext: SearchEmptyContext = { mode: "normal", key: "", tagId: "", pending: false, failed: false };

  it("还没输入的空态不再复述顶部说明（不把顶部那句搬到空态里）", () => {
    const state = resolveSearchEmptyState(baseContext);
    const source = read(readerFiles.searchState);

    expect(state.kind).toBe("idle");
    expect(state.description).toBe("");
    expect(findRemovedCopy(source)).toEqual([]);
  });
});

describe("反向断言：空态 / 错误态 / 加载态的引导文案仍在", () => {
  const baseContext: SearchEmptyContext = { mode: "normal", key: "", tagId: "", pending: false, failed: false };

  it("加载态：等待说明仍在，且不弹下一步按钮", () => {
    const normal = resolveSearchEmptyState({ ...baseContext, key: "nuxt", pending: true });
    const ai = resolveSearchEmptyState({ ...baseContext, mode: "ai", key: "怎么部署", pending: true });

    expect(normal.kind).toBe("loading");
    expect(normal.title).toContain("正在搜索");
    expect(normal.description).toContain("请稍候");
    expect(ai.description).toContain("请稍候");
    expect(normal.actions).toEqual([]);
  });

  it("错误态：失败原因与下一步指引仍在", () => {
    const state = resolveSearchEmptyState({ ...baseContext, key: "nuxt", failed: true });

    expect(state.kind).toBe("error");
    expect(state.title).toBe("搜索暂时不可用");
    expect(state.description.length).toBeGreaterThan(0);
    expect(state.hints.length).toBeGreaterThan(0);
    expect(state.actions).toEqual(["retry", "clear-filters", "switch-mode", "open-home"]);
  });

  it("智能搜索没输入 / 确实没匹配：引导仍在，不是一刀切成空白", () => {
    const needsQuery = resolveSearchEmptyState({ ...baseContext, mode: "ai" });
    const noMatch = resolveSearchEmptyState({ ...baseContext, key: "nuxt" });

    expect(needsQuery.kind).toBe("ai-needs-query");
    expect(needsQuery.description).toContain("自然语言");
    expect(needsQuery.hints.length).toBeGreaterThan(0);
    expect(noMatch.kind).toBe("no-match");
    expect(noMatch.hints.length).toBeGreaterThan(0);
    expect(noMatch.actions).toContain("clear-filters");
  });

  it("搜索页仍把空态的 title / description / hints 与下一步按钮交给统一空态组件", () => {
    const template = read(readerFiles.searchPage);

    expect(template).toContain(':title="emptyState.title"');
    expect(template).toContain(':description="emptyState.description"');
    expect(template).toContain(':hints="emptyState.hints"');
    expect(template).toContain("emptyActions");
  });
});
