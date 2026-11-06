import { describe, expect, it } from "vitest";

import { SPRING_PRESETS } from "~/utils/motion";

import {
  collectAppSources,
  findBareSpringNumbers,
  findUnresolvedSpringConsumers,
  type SourceFile,
} from "./springAudit";

/**
 * 弹簧动效的两条结构门禁（ADR-0012 后续修订 2026-11-07，grill §三 3.9）。
 *
 * 为什么需要它们：`reduced-motion.test.ts` 只扫 CSS 的 `@keyframes` / `animation:`，
 * JS 弹簧完全在既有门禁视野之外 —— 这是本批新开的一个动效入口。所以这里补两条
 * **静态**断言（扫 frontend/app 下的 .vue / .ts，不引 Playwright、不做截图比对）：
 *
 *   门禁一：组件里出现裸 `stiffness` / `damping` 数字即红（参数只能来自 `SPRING_*`）；
 *   门禁二：引用了弹簧预设的文件若没有「减少动态 → 落位」分支即红。
 *
 * 两条都按仓库既有 canary 习惯来证明有牙齿：**喂人造违规文件断言红、喂合规文件断绿**。
 * 尤其门禁二：本票不落任何组件（消费者在后续票），真实仓库此刻没有消费者，
 * 只靠"真实仓库全绿"这条断言它会永远绿 —— 所以人造用例是这条门禁的主要证据。
 */
const appSources = collectAppSources();

/** 人造源文件（不落盘）：门禁的"喂违规"输入，路径模仿真实消费者。 */
function source(file: string, ...lines: string[]): SourceFile {
  return { file, text: lines.join("\n") };
}

describe("门禁一：组件里不得出现裸 stiffness / damping 数字", () => {
  it("真实仓库为绿，且审计确实扫到了组件源码（不是「扫到 0 个文件」式的假绿）", () => {
    expect(appSources.length).toBeGreaterThan(100);
    expect(appSources.filter((entry) => entry.file.endsWith(".vue")).length).toBeGreaterThan(50);
    expect(appSources.some((entry) => entry.file === "app/utils/motion.ts")).toBe(true);

    expect(findBareSpringNumbers(appSources)).toEqual([]);
  });

  it("canary 红：.vue 里内联 stiffness / damping 数字 → 按行报错", () => {
    const violations = findBareSpringNumbers([
      source(
        "app/components/outline/OutlineTree.vue",
        '<script setup lang="ts">',
        'const dotSpring = { type: "spring", stiffness: 200, damping: 26 };',
        "</script>",
      ),
    ]);

    expect(violations).toHaveLength(2);
    expect(violations.map((violation) => violation.file)).toEqual([
      "app/components/outline/OutlineTree.vue",
      "app/components/outline/OutlineTree.vue",
    ]);
    expect(violations.map((violation) => violation.line)).toEqual([2, 2]);
    expect(violations[0].reason).toContain("stiffness");
    expect(violations[0].reason).toContain("motion.ts");
    expect(violations[1].reason).toContain("damping");
  });

  it("canary 绿：同一处改走预设计算 → 不报错", () => {
    expect(
      findBareSpringNumbers([
        source(
          "app/components/outline/OutlineTree.vue",
          '<script setup lang="ts">',
          "const dotTransition = resolveSpringMotion(SPRING_DOT, prefersReducedMotion.value);",
          "</script>",
        ),
      ]),
    ).toEqual([]);
  });

  it("精度：物理配置的字段声明与读取不算「裸弹簧数字」（不误伤 decoCharm 那类模块）", () => {
    expect(
      findBareSpringNumbers([
        source(
          "app/utils/decoCharm.ts",
          "interface RopeConfig {",
          "  damping: number;",
          "  stiffness: number;",
          "}",
          "const k = config.stiffness * config.damping;",
        ),
      ]),
    ).toEqual([]);
  });

  it("motion.ts 是唯一允许写死参数的地方（预算本体）", () => {
    expect(
      findBareSpringNumbers([
        source(
          "app/utils/motion.ts",
          "export const SPRING_DOT = Object.freeze({ stiffness: 180, damping: 24, mass: 0.8 });",
        ),
      ]),
    ).toEqual([]);
  });

  it("canary 红：抄进组件目录下的 .ts（组合式函数）也算违规，不只看 .vue", () => {
    const violations = findBareSpringNumbers([
      source("app/components/inbox/useNeckSpring.ts", "export const neck = { stiffness: 200 };"),
    ]);

    expect(violations).toHaveLength(1);
    expect(violations[0].file).toBe("app/components/inbox/useNeckSpring.ts");
    expect(violations[0].line).toBe(1);
  });
});

describe("门禁二：引用弹簧预设的文件必须有「减少动态 → 落位」分支", () => {
  it("真实仓库为绿（此刻还没有消费者，所以这条门禁的证据在下面的 canary 里）", () => {
    expect(appSources.length).toBeGreaterThan(100);
    expect(findUnresolvedSpringConsumers(appSources)).toEqual([]);
  });

  it("canary 红：直接 useSpring(SPRING_DOT) 套预设、没有落位分支", () => {
    const violations = findUnresolvedSpringConsumers([
      source(
        "app/components/outline/OutlineTree.vue",
        '<script setup lang="ts">',
        "const x = useSpring(SPRING_DOT);",
        "</script>",
      ),
    ]);

    expect(violations).toHaveLength(1);
    expect(violations[0].file).toBe("app/components/outline/OutlineTree.vue");
    expect(violations[0].line).toBe(2);
    expect(violations[0].reason).toContain("resolveSpringMotion");
  });

  it("canary 绿：同一处经 resolveSpringMotion 接入 → 不报错", () => {
    expect(
      findUnresolvedSpringConsumers([
        source(
          "app/components/outline/OutlineTree.vue",
          '<script setup lang="ts">',
          "const dotTransition = resolveSpringMotion(SPRING_DOT, prefersReducedMotion.value);",
          "</script>",
        ),
      ]),
    ).toEqual([]);
  });

  it("canary 红：把减少动态写死成 false 等于没有落位分支", () => {
    const violations = findUnresolvedSpringConsumers([
      source(
        "app/components/inbox/InboxSubNav.vue",
        "const neck = resolveSpringMotion(SPRING_NECK, false);",
      ),
    ]);

    expect(violations).toHaveLength(1);
    expect(violations[0].reason).toContain("SPRING_NECK");
  });

  it("四组预设全部在扫描面内（新增预设也自动进审计）", () => {
    for (const name of Object.keys(SPRING_PRESETS)) {
      const preset = `SPRING_${name.toUpperCase()}`;
      const violations = findUnresolvedSpringConsumers([
        source(`app/components/x/${name}.vue`, `const t = ${preset};`),
      ]);

      expect(violations.map((violation) => violation.reason), preset).toHaveLength(1);
    }
  });

  it("注释里提到预设不算引用（不靠「随手写个字就变红」制造噪音）", () => {
    expect(
      findUnresolvedSpringConsumers([
        source(
          "app/components/outline/OutlineTree.vue",
          "/* 圆点用 SPRING_DOT 驱动，弧线用 SPRING_TRAVEL */",
          "// SPRING_NECK 与这条无关",
          "const x = 1;",
        ),
      ]),
    ).toEqual([]);
  });

  it("motion.ts 本身引用预设是定义，不是消费", () => {
    expect(
      findUnresolvedSpringConsumers([
        source("app/utils/motion.ts", "export const SPRING_PRESETS = { dot: SPRING_DOT };"),
      ]),
    ).toEqual([]);
  });
});
