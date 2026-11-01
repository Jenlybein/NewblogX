import { describe, expect, it } from "vitest";
import {
  ApiBusinessError,
  ApiTimeoutError,
  isApiBusinessError,
  isApiTimeoutError,
  isAuthLikeError,
  shouldRetryAuth,
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

// #98：刷新令牌后的自动重试**只能用于读请求**。
//
// 写请求重放会重复执行副作用——线上实测一次点赞产生两次 PUT /digg，
// 用户看到的是第一次的失败，服务端却已经写了第二遍（进而撞唯一索引）。
// 后端业务码只有 0/1001/1002 三个值，无法区分"未登录"与"参数错误"，
// 所以判别依据不能靠错误码，只能靠**请求方法**这一侧的信息。
describe("shouldRetryAuth", () => {
  const authError = () => new ApiBusinessError("登录状态已过期，请重新登录", 1001);

  it("retries idempotent reads after a refresh", () => {
    expect(shouldRetryAuth("GET", authError())).toBe(true);
  });

  it("never retries writes, so a side effect cannot run twice", () => {
    // 这是本票的核心：写请求一律不重试。
    expect(shouldRetryAuth("POST", authError())).toBe(false);
    expect(shouldRetryAuth("PUT", authError())).toBe(false);
    expect(shouldRetryAuth("DELETE", authError())).toBe(false);
    expect(shouldRetryAuth("PATCH", authError())).toBe(false);
  });

  it("still honours the explicit opt-out", () => {
    expect(shouldRetryAuth("GET", authError(), false)).toBe(false);
  });

  it("does not retry when the failure is not auth-like at all", () => {
    expect(shouldRetryAuth("GET", new Error("参数错误"))).toBe(false);
  });
});
