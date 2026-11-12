// 图形验证码答案怎么进请求体：站点启用 `site.login.captcha` 后，后端
// middleware.CaptchaMiddleware 会在进 handler 之前读 `captcha_id` + `captcha_code`，
// 缺一个就 400。所以这三个服务函数必须把答案摊平进 body，而不是包一层对象
// （包一层后端读不到），也不用 "captcha" 之类的名字（后端只认这两个键）。
import { beforeEach, describe, expect, it, vi } from "vitest";

const requestMock = vi.hoisted(() => vi.fn(async () => ({ id: "code-1" })));

function stubRequest() {
  vi.stubGlobal("useNuxtApp", () => ({ $api: { request: requestMock } }));
}

/** 取这次请求的 body。 */
function bodyOf(): Record<string, unknown> {
  const call = requestMock.mock.calls[0] as unknown as [string, { body: Record<string, unknown> }];
  return call[1].body;
}

describe("图形验证码进请求体", () => {
  beforeEach(() => {
    requestMock.mockClear();
    stubRequest();
  });

  it("发短信验证码时带 captcha_id / captcha_code", async () => {
    const { sendSmsCode } = await import("~/services/auth");
    await sendSmsCode({
      phone: "13800001234",
      purpose: "login",
      captcha: { captcha_id: "cid", captcha_code: "AB12" },
    });

    expect(bodyOf()).toEqual({
      phone: "13800001234",
      type: 1,
      captcha_id: "cid",
      captcha_code: "AB12",
    });
  });

  it("发邮箱验证码时也带", async () => {
    const { sendEmailCode } = await import("~/services/auth");
    await sendEmailCode({
      email: "someone@example.com",
      purpose: "login",
      captcha: { captcha_id: "cid", captcha_code: "AB12" },
    });

    expect(bodyOf()).toEqual({
      email: "someone@example.com",
      type: 4,
      captcha_id: "cid",
      captcha_code: "AB12",
    });
  });

  it("密码登录时也带", async () => {
    const { loginWithPassword } = await import("~/services/auth");
    await loginWithPassword({
      username: "alice",
      password: "secret123",
      captcha: { captcha_id: "cid", captcha_code: "AB12" },
    });

    expect(bodyOf()).toEqual({
      username: "alice",
      password: "secret123",
      captcha_id: "cid",
      captcha_code: "AB12",
    });
  });

  // 站点没开验证码时**一个键都不能多**：后端把这两个字段当可选，多传空串虽然不报错，
  // 但会让"开关关着"和"填了空值"在日志里分不清。
  it("没有验证码答案时请求体里不出现这两个键", async () => {
    const { sendSmsCode } = await import("~/services/auth");
    await sendSmsCode({ phone: "13800001234", purpose: "login" });

    expect(bodyOf()).toEqual({ phone: "13800001234", type: 1 });
    expect("captcha_id" in bodyOf()).toBe(false);
    expect("captcha_code" in bodyOf()).toBe(false);
  });

  // 只填了一半也当没填：宁可让服务端回"图形验证失败"，也不要发一个必然被拒的请求体。
  it("答案只填一半时不带", async () => {
    const { sendSmsCode } = await import("~/services/auth");
    await sendSmsCode({
      phone: "13800001234",
      purpose: "login",
      captcha: { captcha_id: "cid", captcha_code: "" },
    });

    expect("captcha_id" in bodyOf()).toBe(false);
  });
});
