// violenceHints：暴恐类词的**部署侧**注入点（ADR-0032 决策 8 / issue #393 US34）。
//
// 涉政与暴恐类词只进数据库：它们不进仓库、不进默认 yaml、不进 CI 日志。
// 但词表生成脚本仍需要知道"哪些词算暴恐"才能给它们分对类目，所以这份清单
// 放在部署侧的 gitignored 文件里，由构建命令按需读取：
//
//   BLOGX_VIOLENCE_HINTS=/srv/blogx/private/violence-hints.txt \
//     node scripts/content-filter/build-wordlist.mjs --include-violence ...
//
// 文件格式：每行一个词，`#` 开头是注释，空行忽略（与 -filter-import 的 TSV 同一套习惯）。
// 文件缺失或环境变量未设时不报错：这类词兜底归 fraud —— 类目会不准，但词本身
// 仍进库、仍拦截。宁可少一个类目，也不把词写进 git。
import { existsSync, readFileSync } from "node:fs";

/** 环境变量名（部署侧唯一入口，不在仓库里放默认路径）。 */
export const VIOLENCE_HINTS_ENV = "BLOGX_VIOLENCE_HINTS";

/**
 * 读暴恐词清单。
 *
 * 返回 `{ hints, source }`：source 是给人看的来源说明（日志里只说路径，
 * 不打印任何词 —— 与"不进 CI 日志"同一条约束）。
 */
export function loadViolenceHints(env = process.env) {
  const path = String(env[VIOLENCE_HINTS_ENV] ?? "").trim();
  if (!path) {
    return { hints: [], source: `未设置 ${VIOLENCE_HINTS_ENV}（暴恐类兜底归 fraud）` };
  }
  if (!existsSync(path)) {
    return { hints: [], source: `${VIOLENCE_HINTS_ENV}=${path} 不存在（暴恐类兜底归 fraud）` };
  }
  const lines = readFileSync(path, "utf8").split("\n");
  const hints = [];
  for (const line of lines) {
    const word = line.replace(/\r$/, "").trim();
    if (!word || word.startsWith("#") || word.includes("\t")) {
      continue;
    }
    hints.push(word);
  }
  return { hints, source: `${VIOLENCE_HINTS_ENV}=${path}（${hints.length} 条）` };
}
