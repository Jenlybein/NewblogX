import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { stripComments } from "../helpers/designFiles";

/**
 * 项目详情空态不得说破内部状态（spec story 12）。
 *
 * 未展示的项目对读者必须**按不存在处理**：页面既不能说"未公开 / 未发布"，
 * 也不能用"不存在或未公开"这种组合句——后半句已经把内部状态说出来了。
 *
 * 为什么读源码而不是挂载页面：读者站没有页面级挂载测试的先例（依赖
 * useAsyncData / useRoute / 自动导入），`test/gates/` 里已有读源码断言的先例。
 * 判据同时带 canary：只断言现状是恒绿的，判据坏了没人发现。
 */

/** 任何"存在但被隐藏"的说法都会泄漏内部状态，命中即失败。 */
const INTERNAL_STATE_WORDS = ["未公开", "未发布", "尚未公开", "不可见", "不存在或"] as const;

export function findInternalStateLeak(source: string): string[] {
  return INTERNAL_STATE_WORDS.filter((word) => source.includes(word));
}

const projectDir = process.cwd();
const pagePath = join(projectDir, "app/pages/projects/[id].vue");

describe("项目详情空态：对读者按不存在处理", () => {
  it("判据能抓到人造泄漏（canary）", () => {
    expect(findInternalStateLeak("<p>项目不存在或未公开</p>")).toEqual(["未公开", "不存在或"]);
    expect(findInternalStateLeak("<p>尚未公开的项目对读者不可见</p>")).toEqual([
      "未公开",
      "尚未公开",
      "不可见",
    ]);
    expect(findInternalStateLeak("<p>这个项目暂时无法访问。</p>")).toEqual([]);
  });

  it("页面渲染源码里不再出现未发布 / 未公开 / 不可见", () => {
    // 去注释后再扫：注释里可以解释"为什么不能说未公开"，那不是渲染文案。
    const source = stripComments(readFileSync(pagePath, "utf8"));
    expect(findInternalStateLeak(source)).toEqual([]);
  });

  it("空态仍给出中性文案：不存在 + 暂时无法访问", () => {
    const source = readFileSync(pagePath, "utf8");
    expect(source).toContain('"项目不存在"');
    expect(source).toContain('"这个项目暂时无法访问。"');
  });
});
