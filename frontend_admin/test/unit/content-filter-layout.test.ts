import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { maskContentFilterText } from "~/utils/contentFilterPreview";
import {
  CONTENT_FILTER_WHITELIST_FIELDS,
  CONTENT_FILTER_WORD_FIELDS,
  countTagChanges,
  countWordTableChanges,
  mergeContentFilterPayload,
} from "~/utils/contentFilterSave";
import type {
  ContentFilterRuntimeConfig,
  ContentFilterWord,
} from "~/types/api";

/**
 * 关键词过滤重排（2026-11-27）里两块会算错的纯逻辑：
 *
 *   1. **过滤示例的打星**（`utils/contentFilterPreview.ts`）：词表 6 万条，示例必须在
 *      本地算，不能每敲一个字发一次请求；而"归一化后命中""白名单区间覆盖"两条规则
 *      如果写错，运营会被一个演示骗着把词表改坏。
 *   2. **两页共写一段时各自拥有的字段**（`utils/contentFilterSave.ts`）：后端 PUT
 *      是整段覆盖，任一侧漏带对方字段就会静默清空。
 *
 * 另加一条 CSS 类清单断言：设计稿的 `.cf-*` 没有全站通用类可借（后台用 naive-ui），
 * 所以这些类必须真的被移植进 `content-filter.css` —— 否则模板引用的是空类，
 * 页面能渲染但完全没有设计稿的版式。
 */

const projectDir = process.cwd();

function word(word: string): ContentFilterWord {
  return { word, tier: "block", category: "fraud" };
}

function latestConfig(
  overrides: Partial<ContentFilterRuntimeConfig> = {},
): ContentFilterRuntimeConfig {
  return {
    words: [word("代开发票"), word("办假证"), word("加微信")],
    retain_days: 90,
    whitelist: ["证书", "验证码"],
    whitelist_phrases: ["安全套接层"],
    required_categories: ["politics", "violence"],
    missing_required_categories: [],
    whitelist_stats: { total: 3, words: 2, phrases: 1, duplicates: 0 },
    ...overrides,
  } as ContentFilterRuntimeConfig;
}

describe("过滤示例：按当前词表打星（纯前端演示）", () => {
  it("命中一个词：整段替换成等长星号，命中处数 +1", () => {
    const result = maskContentFilterText("有需要的私我，代开发票，量大从优。", [word("代开发票")]);

    expect(result.masked).toBe("有需要的私我，****，量大从优。");
    expect(result.hits).toBe(1);
    expect(result.masked).not.toBe("有需要的私我，代开发票，量大从优。");
  });

  it("匹配前归一化：全角、空格与标点都折叠（不用为变体逐个加词）", () => {
    const result = maskContentFilterText("１ 氨 丙 酮", [word("1氨丙酮")]);

    expect(result.hits).toBe(1);
    // 被压掉的分隔符也一起打星：替换的是整段原文，不是归一化后的文本。
    expect(result.masked).toBe("*******");
  });

  it("同一个词出现两次算两处命中", () => {
    const result = maskContentFilterText("加微信，加微信", [word("加微信")]);

    expect(result.hits).toBe(2);
    expect(result.masked).toBe("***，***");
  });

  it("词级白名单：整词豁免，命中不外溢到别的文本", () => {
    const result = maskContentFilterText(
      "独立站 代开发票",
      [word("独立"), word("代开发票")],
      ["独立"],
    );

    expect(result.hits).toBe(1);
    expect(result.masked).toBe("独立站 ****");
  });

  it("短语白名单：只豁免被短语完整包住的那次命中，短语外的同名词照旧打星", () => {
    const whitelisted = maskContentFilterText(
      "安全套接层",
      [word("安全套")],
      [],
      ["安全套接层"],
    );
    const bare = maskContentFilterText("安全套", [word("安全套")]);

    expect(whitelisted.hits).toBe(0);
    expect(whitelisted.masked).toBe("安全套接层");
    expect(bare.hits).toBe(1);
    expect(bare.masked).toBe("***");
  });

  it("分段结果按「是否打星」聚合并保持原文顺序", () => {
    const result = maskContentFilterText("发代开发票", [word("代开发票")]);

    expect(result.segments.map((segment) => segment.masked)).toEqual([false, true]);
    expect(result.segments.map((segment) => segment.text)).toEqual(["发", "****"]);
  });

  it("词表为空 / 原文为空时不伪造打星", () => {
    expect(maskContentFilterText("代开发票", [])).toEqual({
      masked: "代开发票",
      hits: 0,
      segments: [{ text: "代开发票", masked: false }],
    });
    expect(maskContentFilterText("", [word("代开发票")])).toEqual({
      masked: "",
      hits: 0,
      segments: [],
    });
  });
});

describe("两页共写 content_filter 段的字段归属", () => {
  it("词表页保存：只覆盖 words + retain_days，白名单原样带回", () => {
    const latest = latestConfig({
      whitelist: ["证书"],
      whitelist_phrases: ["安全套接层"],
    });
    const payload = mergeContentFilterPayload(
      latest,
      { words: [word("办假证")], retain_days: 180 },
      CONTENT_FILTER_WORD_FIELDS,
    );

    expect(payload.words).toEqual([word("办假证")]);
    expect(payload.retain_days).toBe(180);
    expect(payload.whitelist).toEqual(["证书"]);
    expect(payload.whitelist_phrases).toEqual(["安全套接层"]);
  });

  it("白名单页保存：只覆盖两份白名单，词表与保留期原样带回", () => {
    const latest = latestConfig({ words: [word("加微信")], retain_days: 30 });
    const payload = mergeContentFilterPayload(
      latest,
      { whitelist: ["证书", "验证码"], whitelist_phrases: ["fork 炸弹"] },
      CONTENT_FILTER_WHITELIST_FIELDS,
    );

    expect(payload.whitelist).toEqual(["证书", "验证码"]);
    expect(payload.whitelist_phrases).toEqual(["fork 炸弹"]);
    expect(payload.words).toEqual([word("加微信")]);
    expect(payload.retain_days).toBe(30);
  });

  it("兄弟页刚存的字段不会被本页旧快照覆盖（读-改-写语义）", () => {
    const fresh = latestConfig({ whitelist: ["新加的词"] });
    // 本页加载时的旧快照：白名单还是旧值。白名单页保存不该把 fresh 的白名单写回旧值。
    const payload = mergeContentFilterPayload(
      fresh,
      { whitelist: ["旧快照里的词"], whitelist_phrases: [] },
      CONTENT_FILTER_WHITELIST_FIELDS,
    );

    // 本页拥有的 whitelist 用本页提交值（它本来就该覆盖）；兄弟页没有的字段不会被碰。
    expect(payload.whitelist).toEqual(["旧快照里的词"]);
    expect(payload.words).toBe(fresh.words);
    expect(payload.retain_days).toBe(90);
    expect(fresh.whitelist).toEqual(["新加的词"]);
  });

  it("请求体四个字段永远齐全（后端对 PUT 是整段覆盖）", () => {
    const payload = mergeContentFilterPayload(latestConfig(), {}, CONTENT_FILTER_WORD_FIELDS);

    expect(Object.keys(payload).sort()).toEqual([
      "auto_dispose",
      "retain_days",
      "whitelist",
      "whitelist_phrases",
      "words",
    ]);
  });

  it("本页某字段为 undefined 时退回最新值；不原地修改入参", () => {
    const latest = latestConfig();
    const payload = mergeContentFilterPayload(latest, { words: undefined }, CONTENT_FILTER_WORD_FIELDS);

    expect(payload.words).toEqual(latest.words);
    expect(latest.words).toEqual([word("代开发票"), word("办假证"), word("加微信")]);
  });

  it("后端 nil 切片 / 缺字段按空数组与默认保留期兜底", () => {
    const latest = {
      ...latestConfig(),
      whitelist: null as unknown as string[],
      whitelist_phrases: null as unknown as string[],
      retain_days: undefined as unknown as number,
    };
    const payload = mergeContentFilterPayload(latest, {}, CONTENT_FILTER_WORD_FIELDS);

    expect(payload.whitelist).toEqual([]);
    expect(payload.whitelist_phrases).toEqual([]);
    expect(payload.retain_days).toBe(90);
  });
});

describe("未保存标记：按「相对上次落库」数改动", () => {
  it("词表：新增 / 删除 / 改档位或类目各算一条", () => {
    const baseline = [word("代开发票"), word("办假证")];
    const next = [
      { ...word("代开发票"), tier: "watch" as const },
      { ...word("办假证") },
      word("加微信"),
    ];
    // 代开发票改了档位 + 加微信新增 = 2；办假证没动。
    expect(countWordTableChanges(baseline, next)).toBe(2);
    // 删掉办假证也算一条。
    expect(countWordTableChanges(baseline, [word("代开发票")])).toBe(1);
    expect(countWordTableChanges(baseline, baseline)).toBe(0);
  });

  it("白名单：新增 + 删除，大小写不敏感", () => {
    expect(countTagChanges(["证书"], ["证书", "验证码"])).toBe(1);
    expect(countTagChanges(["证书", "验证码"], ["证书"])).toBe(1);
    expect(countTagChanges(["证书"], ["证书"])).toBe(0);
    // 只改大小写不算改动（引擎匹配前会折小写）。
    expect(countTagChanges(["fork"], ["Fork"])).toBe(0);
    // 未加载过的空基线（老库字段缺失）不算改动来源。
    expect(countTagChanges(undefined, [])).toBe(0);
  });
});

describe("设计稿 .cf-* 类已移植（不是空类引用）", () => {
  const css = readFileSync(
    join(projectDir, "app", "assets", "css", "content-filter.css"),
    "utf8",
  );

  const PORTED_CLASSES = [
    "cf-page",
    "cf-head",
    "cf-tabs",
    "cf-toolbar",
    "cf-count",
    "cf-sample",
    "cf-strip",
    "cf-layout",
    "cf-cloud",
    "cf-word",
    "cf-legend",
    "cf-empty",
    "cf-more",
    "cf-dirty",
    "cf-aside",
    "cf-panel",
    "cf-meter",
    "cf-note",
    "cf-pair",
    "cf-tagbox",
    "cf-tag",
    "cf-scope",
    "cf-diff",
  ];

  it("设计稿里的 .cf-* 版式类逐个落在样式表里", () => {
    for (const className of PORTED_CLASSES) {
      expect(css, `缺少 .${className}`).toContain(`.${className}`);
    }
  });

  it("样式表只消费语义 token，不出现裸色值", () => {
    // 门禁在 scripts/check-design-tokens.mjs，这里再做一次廉价自检：
    // 裸 hex / rgb() 一旦落进这个文件，设计 token 就多了一条来源。
    // 先去掉注释：注释里提到「rgb()」是在解释规则，不是真的写了色值。
    const declarations = css.replace(/\/\*[\s\S]*?\*\//gu, "");
    expect(declarations).not.toMatch(/#[0-9a-fA-F]{3,8}(?![0-9a-zA-Z-])/u);
    expect(declarations).not.toMatch(/rgba?\([^)]*\)/u);
  });
});



