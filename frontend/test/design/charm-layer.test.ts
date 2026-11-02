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
// 裸色值**不在本文件重复审计**：`scripts/check-design-tokens.mjs` 已经扫描
// `app/**` 的 `.vue/.css/.ts`，并带 baseline 与 canary。这里只保留 token 门禁
// 无法知道的一条 —— 挂绳必须消费 `--color-ink-faint`（而不是任意合规 token）。
//
// 扫描器本身也有"永远绿"的风险（文件挪走 / 正则写错 / 遍历空了），
// 所以下面每条断言都配了人造违规用例：先证明它抓得住已知的坏，再去判真源码。

const projectDir = process.cwd();
const appDir = join(projectDir, "app");
const charmComponent = join(appDir, "components/deco/CursorCharm.vue");

/** 挂件层必须同时具备的四条性质；返回缺失项（空数组 = 合规）。 */
function findCharmLayerProblems(source: string): string[] {
  const problems: string[] = [];
  if (!/aria-hidden="true"/u.test(source)) problems.push("缺 aria-hidden=\"true\"（读屏不能读到它）");
  if (!/pointer-events:\s*none/u.test(source)) problems.push("缺 pointer-events: none（不能拦截任何操作）");
  if (!/z-index:\s*9999/u.test(source)) problems.push("缺 z-index: 9999（要压在弹窗与提示条之上）");
  if (!/var\(--color-ink-faint\)/u.test(source)) {
    problems.push("挂绳必须消费 var(--color-ink-faint)");
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
  it("扫描器抓得住人造违规：缺无障碍属性、缺层级、缺语义 token", () => {
    const canary = [
      "<template>",
      '  <div class="charm-layer">',
      '    <svg><path d="M0 0L0 1" stroke="var(--color-ink)" /></svg>',
      "  </div>",
      "</template>",
      "<style>",
      "  .charm-layer { position: fixed; inset: 0; }",
      "</style>",
    ].join("\n");

    const problems = findCharmLayerProblems(canary);

    expect(problems).toContain('缺 aria-hidden="true"（读屏不能读到它）');
    expect(problems).toContain("缺 pointer-events: none（不能拦截任何操作）");
    expect(problems).toContain("缺 z-index: 9999（要压在弹窗与提示条之上）");
    expect(problems).toContain("挂绳必须消费 var(--color-ink-faint)");
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

  it("挂件层同时具备 aria-hidden + pointer-events: none + z-index: 9999", () => {
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

  it("渐隐走快档（motion-fast）：移出即渐隐，而不是挂件在窗外再挂 200ms", () => {
    const source = readCharmSource();

    expect(source.code, source.file).toContain("var(--motion-fast)");
    // 两个方向都要写 transition：CSS 取"变化之后"那条声明，基础声明只管移出。
    expect(source.code, source.file).toContain(".charm-layer--visible");
    expect(source.code, source.file).toContain("transition: opacity");
  });

  it("门控查询串是「可悬停 + 精确指针」，不接受窄屏近似", () => {
    const utils = readFileSync(join(appDir, "utils/decoCharm.ts"), "utf8");

    expect(utils).toContain('"(hover: hover) and (pointer: fine)"');
  });
});
