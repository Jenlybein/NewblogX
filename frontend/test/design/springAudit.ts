import { readFileSync } from "node:fs";
import { join, relative } from "node:path";

import { SPRING_PRESETS, type SpringPresetName } from "~/utils/motion";

import { collectFiles, extensionFilter } from "../helpers/designFiles";

/**
 * 弹簧动效的两条结构门禁的审计逻辑（纯函数，供 `motion-spring-gates.test.ts` 用）。
 *
 * 为什么审计和断言分开：门禁要能被"喂人造违规 → 红、喂合规 → 绿"证明有牙齿，
 * 所以扫描必须是**吃字符串**的纯函数，不能直接长在断言里（那样只能整份仓库跑一次，
 * 无法构造 canary）。形状对齐仓库既有的 `scripts/check-token-parity.mjs`：
 * 扫描器是纯函数，测试负责喂真实仓库与合成违规。
 *
 * 扫描面：`frontend/app` 下的 `.vue` / `.ts`（README 与 docs 不参与），
 * 唯一豁免是动效预算本体 `app/utils/motion.ts`。
 */

/** 一个待审计的源文件；`file` 用相对 `frontend/` 的路径（报错信息直接可读）。 */
export interface SourceFile {
  /** 相对 frontend/ 的路径，如 `app/components/outline/OutlineTree.vue`。 */
  file: string;
  /** 原始源码（注释在审计内部被抹平，见 `blankComments`）。 */
  text: string;
}

/** 一条违规：文件 + 行号 + 人话原因（门禁失败时直接能定位）。 */
export interface SourceViolation {
  file: string;
  line: number;
  reason: string;
}

/** 动效预算本体：唯一允许写死弹簧参数的地方。 */
export const SPRING_BUDGET_FILE = "app/utils/motion.ts";

/**
 * 弹簧参数的键出现在**对象字面量**里、值又是数字字面量。
 *
 * 为什么是这条而不是"出现 stiffness / damping 就算"：这两个词也会作为**物理配置的
 * 类型声明**（`damping: number`）或**读取**（`config.damping`）出现
 * （`app/utils/decoCharm.ts` 那套挂绳物理、生成的 OpenAPI 类型），把它们一并打红
 * 只会逼人加豁免清单 —— 门禁一旦有豁免清单就开始腐烂。这里精确指向"内联弹簧数字"
 * 这一种写法，与 ADR 的口径一致。
 */
const BARE_SPRING_NUMBER = /\b(stiffness|damping)\s*:\s*-?\d/gu;

/** 预设引用：名字由 `SPRING_PRESETS` 的键生成，新增预设自动进扫描面。 */
const SPRING_PRESET_REFERENCE = new RegExp(
  `\\bSPRING_(?:${(Object.keys(SPRING_PRESETS) as SpringPresetName[])
    .map((name) => name.toUpperCase())
    .join("|")})\\b`,
  "u",
);

/** `resolveSpringMotion(SPRING_X, <第二个实参>)` —— 唯一被承认的落位接入方式。 */
const RESOLVER_CALL = /resolveSpringMotion\(\s*(SPRING_[A-Z_]+)\s*,\s*([^)]*)/gu;

/** 第二个实参里必须能看出"减少动态"（`prefersReducedMotion` / `reduced.value` …）。 */
const REDUCED_MOTION_ARGUMENT = /reduced/iu;

/**
 * 把注释换成等长空白（保留换行）。
 *
 * 不用 `stripComments`：它会删掉注释内容、行号随之错位，而门禁的报错必须能直接
 * 跳到那一行。等长空白既让注释里的 `SPRING_TRAVEL` / `stiffness: 200` 不再算数，
 * 又保证行号与原文件一致。
 */
function blankComments(text: string): string {
  const blank = (chunk: string): string => chunk.replace(/[^\n]/gu, " ");

  return text
    .replace(/<!--[\s\S]*?-->/gu, blank)
    .replace(/\/\*[\s\S]*?\*\//gu, blank)
    .replace(/^[ \t]*\/\/.*$/gmu, blank);
}

/** 1-based 行号。 */
function lineAt(text: string, index: number): number {
  return text.slice(0, index).split(/\r?\n/u).length;
}

/**
 * 读取待审计的源文件。
 *
 * @param appRoot 默认 `frontend/app`；canary 可指向 fixture 目录。
 */
export function collectAppSources(appRoot: string = join(process.cwd(), "app")): SourceFile[] {
  const projectDir = process.cwd();

  return collectFiles(appRoot, extensionFilter("vue", "ts"))
    .map((file) => ({
      file: relative(projectDir, file).replace(/\\/gu, "/"),
      text: readFileSync(file, "utf8"),
    }))
    .sort((left, right) => left.file.localeCompare(right.file));
}

/**
 * 门禁一：组件里出现裸 `stiffness` / `damping` 数字。
 *
 * 参数只能来自 `SPRING_*` 预设（组件写 `useSpring(SPRING_TRAVEL)` 或
 * `resolveSpringMotion(SPRING_TRAVEL, …)`），这样"统一调手感"才只需要改一处。
 */
export function findBareSpringNumbers(sources: SourceFile[]): SourceViolation[] {
  const violations: SourceViolation[] = [];

  for (const { file, text } of sources) {
    if (file === SPRING_BUDGET_FILE) continue;

    const code = blankComments(text);
    for (const match of code.matchAll(BARE_SPRING_NUMBER)) {
      violations.push({
        file,
        line: lineAt(code, match.index ?? 0),
        reason: `裸 ${match[1]} 数字：弹簧参数只能来自 ${SPRING_BUDGET_FILE} 的 SPRING_* 预设`,
      });
    }
  }

  return violations;
}

/**
 * 门禁二：引用了弹簧预设、却没有「减少动态 → 落位」分支。
 *
 * 判据是"文件里出现了 `SPRING_*`，且至少有一处 `resolveSpringMotion(SPRING_*, …)`
 * 的第二个实参能看出是减少动态偏好"。把偏好写死成 `false`、或直接
 * `useSpring(SPRING_TRAVEL)` 绕过解析，都算缺分支 —— 这两种写法在 reduce 下都会继续动。
 */
export function findUnresolvedSpringConsumers(sources: SourceFile[]): SourceViolation[] {
  const violations: SourceViolation[] = [];

  for (const { file, text } of sources) {
    if (file === SPRING_BUDGET_FILE) continue;

    const code = blankComments(text);
    const reference = code.match(SPRING_PRESET_REFERENCE);
    if (!reference) continue;

    const resolved = [...code.matchAll(RESOLVER_CALL)].some(
      (call) =>
        SPRING_PRESET_REFERENCE.test(call[1]) && REDUCED_MOTION_ARGUMENT.test(call[2] ?? ""),
    );
    if (resolved) continue;

    violations.push({
      file,
      line: lineAt(code, reference.index ?? 0),
      reason:
        `引用了弹簧预设 ${reference[0]} 但没有「减少动态 → 落位」分支：` +
        `应经 resolveSpringMotion(${reference[0]}, prefersReducedMotion) 解析（reduce 时落位为静止）`,
    });
  }

  return violations;
}
