import { describe, expect, it } from "vitest";
import { ApiBusinessError, isApiBusinessError, isAuthLikeError } from "~/services/http/errors";

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
});
