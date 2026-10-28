import { describe, expect, it } from "vitest";
import {
  ApiBusinessError,
  ApiTimeoutError,
  isApiBusinessError,
  isApiTimeoutError,
  isAuthLikeError,
} from "~/services/http/errors";

describe("http error helpers", () => {
  it("recognizes project business errors", () => {
    const error = new ApiBusinessError("失败", 5001, { detail: "x" }, 400);

    expect(isApiBusinessError(error)).toBe(true);
    expect(error.code).toBe(5001);
    expect(error.statusCode).toBe(400);
  });

  it("treats explicit auth codes and token messages as auth-like failures", () => {
    expect(isAuthLikeError(new ApiBusinessError("未登录", 401))).toBe(true);
    expect(isAuthLikeError(new ApiBusinessError("权限错误", 403))).toBe(true);
    expect(isAuthLikeError(new Error("token expired"))).toBe(true);
    expect(isAuthLikeError(new Error("network down"))).toBe(false);
    expect(isAuthLikeError("未登录")).toBe(false);
  });

  // 超时必须是一个独立类型：客户端靠它区分"网络停了要提示重试"和
  // "用户主动取消不该报错"。否则两者都是通用 abort 错误，界面只能一律报错。
  it("carries path and limit on timeout errors", () => {
    const error = new ApiTimeoutError("/api/articles/1/digg", 15000);

    expect(isApiTimeoutError(error)).toBe(true);
    expect(error.path).toBe("/api/articles/1/digg");
    expect(error.timeoutMs).toBe(15000);
    expect(error.message).toContain("15 秒");
  });

  it("does not mistake a timeout for an auth failure", () => {
    // 超时不触发刷新令牌重试：连不上就是连不上，重试只是让用户多等一轮。
    expect(isAuthLikeError(new ApiTimeoutError("/api/x", 15000))).toBe(false);
  });
});
