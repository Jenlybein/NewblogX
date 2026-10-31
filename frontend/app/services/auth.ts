import type { Schema } from "~/types/contract";
import type { EmailVerifyResponse } from "~/types/api";

export function loginWithPassword(payload: { username: string; password: string }) {
  return useNuxtApp().$api.request<string>("/api/users/login", {
    method: "POST",
    body: payload,
    auth: false,
    retryAuth: false,
  });
}

/** 契约里 `type` 的取值（swag 从 `binding:"oneof=1 2 3 4"` 生成了 enum）。 */
type EmailCodeType = Schema<"internal_auth_adapter_http.SendEmailRequest">["type"];

/**
 * 邮箱验证码的用途 → 契约里 `type` 的取值。
 *
 * 映射只准留在这一个文件里：调用点写用途名（"register"），不写数字。
 * 用数字的代价是**发错信/发不出信**——后端按 type 判断邮箱状态是否匹配
 * （注册/绑定要求邮箱未注册，登录/重置要求已注册，见 backend/app/user_wiring.go
 * 的 emailCodeBridge.Send）。2026-10-31 线上事故就是注册表单写成了 `type: 2`
 * （重置密码），邮箱未注册 → 后端拒绝 → 用户注册永远收不到验证码。
 */
const EMAIL_CODE_TYPE = {
  register: 1,
  resetPassword: 2,
  bindEmail: 3,
  login: 4,
} as const satisfies Record<string, EmailCodeType>;

/** 验证码用途；取值同后端 SendEmailRequest.type 的语义。 */
export type EmailCodePurpose = keyof typeof EMAIL_CODE_TYPE;

export function sendEmailCode(payload: { email: string; purpose: EmailCodePurpose }) {
  return useNuxtApp().$api.request<EmailVerifyResponse>("/api/users/email/verify", {
    method: "POST",
    body: { email: payload.email, type: EMAIL_CODE_TYPE[payload.purpose] },
    auth: false,
    retryAuth: false,
  });
}

export function loginWithEmailCode(payload: { email_id: string; email_code: string }) {
  return useNuxtApp().$api.request<string>("/api/users/email/login", {
    method: "POST",
    body: payload,
    auth: false,
    retryAuth: false,
  });
}

export function registerWithEmail(payload: { pwd: string; email_id: string; email_code: string }) {
  return useNuxtApp().$api.request<string>("/api/users/email/register", {
    method: "POST",
    body: payload,
    auth: false,
    retryAuth: false,
  });
}

export function logoutCurrentSession() {
  return useNuxtApp().$api.request("/api/users/logout", {
    method: "POST",
    auth: true,
    retryAuth: false,
  });
}
