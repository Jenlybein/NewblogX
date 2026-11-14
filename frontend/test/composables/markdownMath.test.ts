import { describe, expect, it } from "vitest";

import { renderArticleMarkdown } from "~/composables/useArticleMarkdown";
import { normalizeMathBlocks, renderMath } from "~/utils/markdownMath";

/**
 * 公式渲染回归。
 *
 * 线上真实故障（2026-11-14，文章《深度学习入门-神经网络和反向传播》）：
 * - 每个 `$$…$$` 公式在页面上显示成 LaTeX 源码（`\sigma(\vec{w}^T \cdot \vec{x})
 *   \tag{3.1}`）；
 * - `$…$` 行内公式塌成散字。
 *
 * 根因是 `markdown-it-katex@2.0.3` 自带的 `katex@0.6`：不认 `\tag`（解析抛错后把源码
 * 原样吐回页面），而且产出的类名是 0.6 那一代的，与站点加载的 0.16 CSS 对不上。
 * 现在改用 `~/utils/markdownMath` 直接调站点依赖的 `katex@0.16`。
 */
describe("公式渲染", () => {
  it("行内 `$…$` 产出 KaTeX 结构（不是散字，也不是源码）", () => {
    const { html } = renderArticleMarkdown("输入向量为 $\\vec{x}$，权重为 $\\vec{w}$。");
    expect(html).toContain('class="katex"');
    // KaTeX 0.16 的类名；0.6 的 `textstyle uncramped` 说明装错了版本。
    expect(html).not.toContain("textstyle uncramped");
    // 没有降级标记 = 真的渲染成功（源码只出现在 MathML 里，读者看不到）。
    expect(html).not.toContain("math-error");
    expect(html).toMatch(/<mover accent="true"><mi>x<\/mi>/u);
  });

  it("块级 `$$…$$` 产出 `katex-display`，且 `\\tag` 生效", () => {
    const { html } = renderArticleMarkdown(
      "$$\ny = \\sigma(\\vec{w}^T \\cdot \\vec{x}) \\quad \\tag{3.1}\n$$",
    );
    expect(html).toContain("katex-display");
    expect(html).toContain('class="tag"');
    expect(html).not.toContain("math-error");
  });

  it("`\\begin{align*}` 多行对齐可渲染（含 tag 跟在环境外）", () => {
    const { html } = renderArticleMarkdown(
      "$$\n\\begin{align*}\n\\frac{d}{dx}\\,\\sigma(x) &= \\frac{e^{-x}}{(1+e^{-x})^{2}} \\\\\n&= \\sigma(x)(1-\\sigma(x))\n\\end{align*} \\tag{3.3}\n$$",
    );
    expect(html).toContain("katex-display");
    expect(html).toContain('class="tag"');
  });

  it("单行 `$$ x $$` 与多行 `$$` 都按块级处理", () => {
    const single = renderArticleMarkdown("$$ x = 1 $$").html;
    const multi = renderArticleMarkdown("$$\nx = 1\n$$").html;
    expect(single).toContain("katex-display");
    expect(multi).toContain("katex-display");
  });

  it("价格写法不被当成公式（闭括号后面不能紧跟数字）", () => {
    const { html } = renderArticleMarkdown("价格 $5 和 $10 之间没有换行就会误判。");
    expect(html).not.toContain('class="katex"');
    expect(html).toContain("$5");
    expect(html).toContain("$10");
  });

  it("转义的 `\\$` 不当分隔符", () => {
    const { html } = renderArticleMarkdown("美元写作 \\$1.5 与 \\$2.5。");
    expect(html).not.toContain('class="katex"');
  });

  it("语法错误的公式降级成 `.math-error`，不再把源码当正文吐出来", () => {
    const broken = renderMath("\\frac{1}{", true);
    expect(broken).toContain("math-error");
    expect(broken).toContain('title="');
    // 降级仍然保留源码文本（读者至少能看出式子想写什么）。
    expect(broken).toContain("\\frac{1}{");
    expect(broken).not.toContain("katex-display");
  });

  it("合法公式走 KaTeX，不产生降级标记", () => {
    const ok = renderMath("x^2 + y^2 = z^2", false);
    expect(ok).toContain('class="katex"');
    expect(ok).not.toContain("math-error");
  });

  it("消毒后 MathML 仍在：否则公式只剩 aria-hidden 的视觉副本，读屏器读不到", () => {
    const { html } = renderArticleMarkdown("$$\nx = 1 \\tag{1}\n$$");
    expect(html).toContain("<math");
    expect(html).toContain("katex-mathml");
  });
});

/**
 * `$$…$$` 归一。
 *
 * 真实正文里两种写坏的分隔：开在引用 / 列表里（`>    $$`），闭的却在行首。
 * 按 Markdown 容器语义，闭括号那行不属于引用 → 引用里没有闭括号 → 后面的裸 `$$`
 * 去配对下一个公式，把中间整段正文当公式内容吞掉（错误信息里能看到 `#` 标题、
 * `>` 引用号）。归一的目的只是让"成对的分隔行"真的成对。
 */
describe("`$$…$$` 归一", () => {
  it("引用里开、行首闭：归成一行，且留在引用里", () => {
    const source = [">    接下来看公式：", ">    $$", "a &= b \\\\", "\\tag{3.1}", "$$", ">    以上。"];
    const out = normalizeMathBlocks(source.join("\n")).split("\n");
    expect(out[1]).toBe(">    $$ a &= b \\\\ \\tag{3.1} $$");
    // 6 行 → 3 行：两条分隔 + 两行内容折叠成一行。
    expect(out).toHaveLength(3);
    expect(out[0]).toBe(">    接下来看公式：");
    expect(out[2]).toBe(">    以上。");
  });

  it("正常的顶层多行公式也归一成一行（KaTeX 本来就接受单行）", () => {
    const out = normalizeMathBlocks("$$\nx = 1\n$$");
    expect(out).toBe("$$ x = 1 $$");
  });

  it("已经是单行的公式不动", () => {
    const out = normalizeMathBlocks("$$ x = 1 $$");
    expect(out).toBe("$$ x = 1 $$");
  });

  it("配对错乱时也不吞正文：跨度超限就原样留下", () => {
    const long = ["$$", ...Array.from({ length: 70 }, (_, i) => `line ${i}`), "$$"];
    const out = normalizeMathBlocks(long.join("\n"));
    // 没归一 → 内容行都还在（交给 markdown-it 当普通文本，不会整段消失）。
    expect(out.split("\n")).toHaveLength(long.length);
  });

  it("引用的前缀被逐行剥掉，不会混进公式内容", () => {
    const out = normalizeMathBlocks(["> $$", "> \\frac{1}{2}", "> $$"].join("\n"));
    expect(out).toBe("> $$ \\frac{1}{2} $$");
  });
});