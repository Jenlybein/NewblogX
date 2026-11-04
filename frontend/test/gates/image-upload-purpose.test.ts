import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * #216 上传用途门禁：五个封面/头像入口必须显式声明用途，私信发图必须显式
 * 声明"不声明用途"（`purpose: null`，任意比例照片不能被 16:9 规则误判）。
 *
 * `uploadImageByTask` 默认 cover 是兼容旧调用的兜底，但入口不应依赖兜底：
 * 新增调用点若忘了传，这条门禁会红，逼调用点先想清楚用途。
 *
 * 判据读源码而不是挂页面：两端 services/image.ts 是同源形态，入口散落在
 * composable 与 .vue 页面里，逐页挂载的成本远高于静态断言；`test/gates/`
 * 已有同类先例。带 canary：判据本身坏了也要能红。
 */

/** 入口相对仓库根的路径 → 该入口应声明的用途（null = 不声明）。 */
const ENTRY_POINTS: Array<{ file: string; purpose: "cover" | "avatar" | null }> = [
  // 读者站：文章封面、头像、私信图片
  { file: "frontend/app/composables/useStudioArticle.ts", purpose: "cover" },
  { file: "frontend/app/pages/studio/settings.vue", purpose: "avatar" },
  { file: "frontend/app/composables/useInboxCenter.ts", purpose: null },
  // 后台：轮播封面、合集封面、项目封面
  { file: "frontend_admin/app/pages/media/index.vue", purpose: "cover" },
  { file: "frontend_admin/app/components/articles/CollectionEditorOverlay.vue", purpose: "cover" },
  { file: "frontend_admin/app/components/articles/CollectionEditorOverlay.vue", purpose: "cover" },
];

const repoRoot = join(process.cwd(), "..");

/** 从源码里抽出所有 `uploadImageByTask(` 调用的第二个参数（选项对象）文本。 */
export function findUploadOptions(source: string): string[] {
  const out: string[] = [];
  const marker = "uploadImageByTask(";
  let from = 0;
  while (true) {
    const start = source.indexOf(marker, from);
    if (start < 0) break;
    // 取到匹配的右括号：选项里可能出现嵌套对象/回调，简单括号计数足够。
    let depth = 0;
    let end = start + marker.length;
    for (; end < source.length; end += 1) {
      const ch = source[end];
      if (ch === "(") depth += 1;
      if (ch === ")") {
        if (depth === 0) break;
        depth -= 1;
      }
    }
    const call = source.slice(start, end);
    const comma = call.indexOf(",");
    out.push(comma < 0 ? "" : call.slice(comma + 1));
    from = end + 1;
  }
  return out;
}

export function purposeOf(optionsText: string): "cover" | "avatar" | null | undefined {
  if (/purpose\s*:\s*null/.test(optionsText)) return null;
  const cover = /purpose\s*:\s*["']cover["']/.test(optionsText);
  const avatar = /purpose\s*:\s*["']avatar["']/.test(optionsText);
  if (cover && !avatar) return "cover";
  if (avatar && !cover) return "avatar";
  return undefined;
}

describe("上传用途门禁判据（canary）", () => {
  it("能识别显式用途 / 不声明 / 漏传三种情形", () => {
    expect(findUploadOptions('const x = uploadImageByTask(file, { purpose: "cover" })')).toEqual([' { purpose: "cover" }']);
    expect(purposeOf(' { onStage: fn, purpose: "avatar" }')).toBe("avatar");
    expect(purposeOf(" { purpose: null })")).toBeNull();
    expect(purposeOf(" { onStage: fn }")).toBeUndefined();
    expect(purposeOf("")).toBeUndefined();
  });
});

describe("六个上传入口的 purpose 声明", () => {
  for (const entry of ENTRY_POINTS) {
    it(`${entry.file} → ${entry.purpose ?? "不声明"}`, () => {
      const source = readFileSync(join(repoRoot, entry.file), "utf8");
      const optionsList = findUploadOptions(source);
      expect(optionsList.length, `${entry.file} 应有且仅有一个上传调用点`).toBeGreaterThan(0);
      for (const options of optionsList) {
        expect(purposeOf(options), `${entry.file} 的上传调用必须显式声明用途`).toBe(entry.purpose);
      }
    });
  }
});
