import { existsSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

// 光标挂件层的设计门禁（spec #35 / 票 #40）。
//
// 为什么是源码扫描而不是渲染断言：这一层的正确性几乎全是"不可见"的性质 ——
// 不拦截、读屏读不到、压在一切之上、颜色只吃语义 token。渲染断言只能证明
// 某一次渲染的样子，证明不了下一个改这个组件的人不会把 `pointer-events: none`
// 顺手删掉。静态扫描把四条性质钉在源码里，改坏了立刻红。
//
// 扫描器本身也有"永远绿"的风险（文件挪走 / 正则写错 / 遍历空了），
// 所以下面每条断言都配了人造违规用例：先证明它抓得住已知的坏，再去判真源码。

const projectDir = process.cwd();
const appDir = join(projectDir, "app");
const charmComponent = join(appDir, "components/deco/CursorCharm.vue");

/** 一行命中：行号 + 命中的字面量。 */
interface Hit {
  line: number;
  match: string;
}

function lineOf(source: string, index: number): number {
  return source.slice(0, index).split(/\r?\n/u).length;
}

// 裸色值：设计 token 门禁（scripts/check-design-tokens.mjs）拦的同一族。
// 例外只有一个 —— 全透明的 `transparent` 关键字不是色值，fade 用它。
const BARE_COLOR_RE = /#[0-9a-f]{3,8}\b|\b(?:rgb|rgba|hsl|hsla)\s*\(/giu;

/** 找出源码里的裸色值（注释不参与审计，调用方先剥注释）。 */
function findBareColors(source: string): Hit[] {
  const hits: Hit[] = [];
  for (const match of source.matchAll(BARE_COLOR_RE)) {
    hits.push({ line: lineOf(source, match.index), match: match[0] });
  }
  return hits;
}

/** 挂件层必须同时具备的四条性质；返回缺失项（空数组 = 合规）。 */
function findCharmLayerProblems(source: string): string[] {
  const problems: string[] = [];
  if (!/aria-hidden="true"/u.test(source)) problems.push("缺 aria-hidden=\"true\"（读屏不能读到它）");
  if (!/pointer-events:\s*none/u.test(source)) problems.push("缺 pointer-events: none（不能拦截任何操作）");
  if (!/z-index:\s*9999/u.test(source)) problems.push("缺 z-index: 9999（要压在弹窗与提示条之上）");
  if (!/var\(--color-ink-faint\)/u.test(source)) {
    problems.push("挂绳必须消费 var(--color-ink-faint)");
  }
  for (const hit of findBareColors(source)) {
    problems.push(`第 ${hit.line} 行出现裸色值 ${hit.match}`);
  }
  return problems;
}

/** 读组件源码；注释里的说明不参与审计（只认真实声明）。 */
function readCharmSource(): { file: string; text: string; code: string } {
  const text = existsSync(charmComponent) ? readFileSync(charmComponent, "utf8") : "";
  return {
    file: relative(projectDir, charmComponent).replace(/\\/gu, "/"),
    text,
    code: text.replace(/<!--[\s\S]*?-->/gu, "").replace(/\/\*[\s\S]*?\*\//gu, ""),
  };
}

describe("光标挂件层的设计门禁（canary 保证扫描器不是永远绿）", () => {
  it("扫描器抓得住人造违规：缺无障碍属性、缺层级、写裸色值", () => {
    const canary = [
      "<template>",
      '  <div class="charm-layer">',
      '    <svg><path d="M0 0L0 1" stroke="#8a8378" /></svg>',
      "  </div>",
      "</template>",
      "<style>",
      "  .charm-layer { position: fixed; inset: 0; }",
      "  .charm-layer__tether { stroke: rgba(0, 0, 0, 0.2); }",
      "</style>",
    ].join("\n");

    const problems = findCharmLayerProblems(canary);

    expect(problems).toContain('缺 aria-hidden="true"（读屏不能读到它）');
    expect(problems).toContain("缺 pointer-events: none（不能拦截任何操作）");
    expect(problems).toContain("缺 z-index: 9999（要压在弹窗与提示条之上）");
    expect(problems.some((problem) => problem.includes("裸色值 #8a8378"))).toBe(true);
    expect(problems.some((problem) => problem.includes("rgba("))).toBe(true);
    // 行号要指向真实位置（第 3 行那条 hex），否则定位信息没用。
    expect(findBareColors(canary).map((hit) => hit.line)).toEqual([3, 8]);
  });

  it("消费语义 token 的写法算合规（不误报）", () => {
    const ok = [
      '<div class="charm-layer" aria-hidden="true">',
      '  <svg><path stroke="var(--color-ink-faint)" opacity="0.5" /></svg>',
      "</div>",
      "<style>",
      "  .charm-layer { pointer-events: none; z-index: 9999; }",
      "  .charm-layer--visible { opacity: 1; }",
      "</style>",
    ].join("\n");

    expect(findCharmLayerProblems(ok)).toEqual([]);
  });

  it("挂件组件真的存在且被扫到了（防止文件挪走后全绿）", () => {
    const source = readCharmSource();

    expect(existsSync(charmComponent), "挂件组件不存在：app/components/deco/CursorCharm.vue").toBe(true);
    expect(source.text.length).toBeGreaterThan(500);
  });

  it("挂件层同时具备 aria-hidden + pointer-events: none + z-index: 9999，且不含裸色值", () => {
    const source = readCharmSource();

    expect(findCharmLayerProblems(source.code), source.file).toEqual([]);
  });

  it("挂绳颜色只吃极淡档语义 token（暗色/亮色两态各有一份取值，不写死）", () => {
    const source = readCharmSource();

    expect(source.code, source.file).toContain("var(--color-ink-faint)");
    // tokens.css 里这一档的既有定义就是"仅大字 / 装饰 / 控件边界"。
    const tokens = readFileSync(join(appDir, "assets/css/tokens.css"), "utf8");
    expect(tokens).toContain("--color-ink-faint");
  });

  it("淡入只动 opacity 且用全站唯一缓动（base 档 200ms）", () => {
    const source = readCharmSource();

    expect(source.code, source.file).toContain("var(--motion-base)");
    expect(source.code, source.file).toContain("var(--motion-ease)");
    // 不做位移/缩放式的入场：装饰层只淡入，免得读者瞥见东西在飞。
    expect(source.code, source.file).not.toMatch(/@keyframes/u);
  });

  it("门控查询串是「可悬停 + 精确指针」，不接受窄屏近似", () => {
    const utils = readFileSync(join(appDir, "utils/decoCharm.ts"), "utf8");

    expect(utils).toContain('"(hover: hover) and (pointer: fine)"');
  });
});
