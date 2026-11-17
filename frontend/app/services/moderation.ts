import type { ModerationSummary, ModerationAppealItem } from "~/types/api";

/**
 * 违规记录与申诉（ADR-0035）。
 *
 * 单独一个 service 文件而不是塞进 `studio.ts`：这两件事的读面在
 * `/api/moderation/*`，权限与生命周期都独立于账号资料。
 */

/** 读自己的有效分与违规时间线。 */
export function getMyViolations() {
  return useNuxtApp().$api.request<ModerationSummary>("/api/moderation/me", { method: "GET" });
}

/**
 * 对一条处罚提交申诉（7 天窗口、一次一条，服务端校验）。
 *
 * event_id 是雪花 ID：契约里印成 string（超出 JS 安全整数），所以这里也是 string。
 */
export function submitViolationAppeal(payload: { event_id: string; reason: string }) {
  return useNuxtApp().$api.request<ModerationAppealItem>("/api/moderation/me/appeals", {
    method: "POST",
    body: payload,
  });
}

