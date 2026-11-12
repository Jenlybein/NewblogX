import type { Schema } from "~/types/contract";
import type { EmailVerifyResponse } from "~/types/api";

/**
 * 图形验证码答案。
 *
 * 站点打开 `site.login.captcha` 之后，**发码与密码登录**都必须带上它，
 * 否则 `middleware.CaptchaMiddleware` 会在进 handler 之前直接 400。
 * 开关关着时不传即可（后端把这两个字段当可选）。
 */
export type CaptchaProof = { captcha_id: string; captcha_code: string };

/** 图片验证码：`base64` 已经是可以直接放进 `<img src>` 的 data URI。 */
export type ImageCaptcha = { captcha_id: string; base64: string };

/** 取一张图形验证码（一次性：校验成功即失效，所以每次提交后要重新取）。 */
export function getImageCaptcha() {
  return useNuxtApp().$api.request<ImageCaptcha>("/api/imagecaptcha", {
    auth: false,
    retryAuth: false,
  });
}

/** 把验证码答案摊进请求体；没填齐就当没传，由服务端决定是否必填。 */
function captchaFields(proof?: CaptchaProof): Partial<CaptchaProof> {
  if (!proof?.captcha_id || !proof.captcha_code) {
    return {};
  }
  return { captcha_id: proof.captcha_id, captcha_code: proof.captcha_code };
}

export function loginWithPassword(payload: { username: string; password: string; captcha?: CaptchaProof }) {
  return useNuxtApp().$api.request<string>("/api/users/login", {
    method: "POST",
    body: {
      username: payload.username,
      password: payload.password,
      ...captchaFields(payload.captcha),
    },
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

export function sendEmailCode(payload: { email: string; purpose: EmailCodePurpose; captcha?: CaptchaProof }) {
  return useNuxtApp().$api.request<EmailVerifyResponse>("/api/users/email/verify", {
    method: "POST",
    body: {
      email: payload.email,
      type: EMAIL_CODE_TYPE[payload.purpose],
      ...captchaFields(payload.captcha),
    },
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

/** 短信验证码的用途 → 契约里 `type` 的取值（与后端 platform/sms 的 Kind 对齐）。 */
const SMS_CODE_TYPE = {
  login: 1,
  passwordProof: 3,
} as const;

/** 短信验证码用途。 */
export type SmsCodePurpose = keyof typeof SMS_CODE_TYPE;

/**
 * 发送手机验证码。
 *
 * 与邮件那套的差别：手机验证码由阿里云生成并核验，所以没有 `email_id` 这种
 * 一次性记录 ID，登录时直接把 phone + code 一起提交。
 */
export function sendSmsCode(payload: { phone: string; purpose: SmsCodePurpose; captcha?: CaptchaProof }) {
  return useNuxtApp().$api.request("/api/users/phone/verify", {
    method: "POST",
    body: {
      phone: payload.phone,
      type: SMS_CODE_TYPE[payload.purpose],
      ...captchaFields(payload.captcha),
    },
    auth: false,
    retryAuth: false,
  });
}

/** 手机号 + 短信验证码登录；首次登录即注册。 */
export function loginWithPhone(payload: { phone: string; code: string }) {
  return useNuxtApp().$api.request<string>("/api/users/phone", {
    method: "POST",
    body: payload,
    auth: false,
    retryAuth: false,
  });
}

/**
 * 给尚无密码的账号设置首个密码。
 *
 * 需要再走一次短信复验：只凭"已登录"就允许设密码，会让一个被盗的会话
 * 把临时凭证升级成永久密码凭证。
 */
export function setAccountPassword(payload: { phone: string; code: string; new_password: string }) {
  return useNuxtApp().$api.request("/api/users/password/set", {
    method: "POST",
    body: payload,
    auth: true,
    retryAuth: false,
  });
}
