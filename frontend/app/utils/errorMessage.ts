/**
 * 错误文案的唯一取值点。
 *
 * 同一份 `reason instanceof Error ? reason.message : "…"` 以前散落在几十个
 * catch 块里：口径一次改（比如空文案要回落）就得逐处改，还容易漏。
 * 这里只做一件事——从 unknown 里拿一句能直接展示的话，拿不到就用调用方给的兜底。
 */
export function errorMessage(reason: unknown, fallback: string): string {
  // 空 message 也算"拿不到"：toast 里显示一个空气泡比兜底文案更糟。
  return reason instanceof Error && reason.message ? reason.message : fallback;
}
