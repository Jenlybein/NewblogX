import { describe, expect, it } from "vitest";
import { withRequestTimeout } from "~/services/http/client";
import { ApiTimeoutError, isApiTimeoutError } from "~/services/http/errors";

// 真实的 $fetch 在 signal 被 abort 时会 reject，这里必须复现这一点：
// 否则"永不返回"的桩会连超时都卡不住（promise 既不 resolve 也不 reject），
// 测试就变成在等一个永远不来的结果，而不是在验证超时。
function pendingUntilAborted<T>() {
  return (_signal: AbortSignal) =>
    new Promise<T>((_resolve, reject) => {
      _signal.addEventListener("abort", () => reject(new Error("aborted")));
    });
}

describe("withRequestTimeout", () => {
  // 这条是整套超时的核心：连接停顿（不回也不断）时必须 reject，
  // 否则调用方的按钮会一直转圈，而服务端可能压根没收到请求。
  it("让停顿的请求在超时后 reject，且报的是超时而不是通用 abort", async () => {
    const pending = withRequestTimeout("/api/articles/1/digg", 20, undefined, pendingUntilAborted());

    let caught: unknown;
    try {
      await pending;
    } catch (error) {
      caught = error;
    }

    expect(isApiTimeoutError(caught)).toBe(true);
    expect(caught).toBeInstanceOf(ApiTimeoutError);
    expect((caught as ApiTimeoutError).path).toBe("/api/articles/1/digg");
  });

  it("正常返回的请求不受超时包装影响", async () => {
    await expect(withRequestTimeout("/api/x", 1000, undefined, async () => "ok")).resolves.toBe("ok");
  });

  // 上层取消（切页面、组件卸载）要立刻生效，且不能被误判成超时 ——
  // 超时要提示"重试"，用户主动取消则不该弹任何错误。
  it("上层取消时立刻中止，且不误判为超时", async () => {
    const outer = new AbortController();
    const pending = withRequestTimeout("/api/x", 10_000, outer.signal, pendingUntilAborted());

    outer.abort();

    let caught: unknown;
    try {
      await pending;
    } catch (error) {
      caught = error;
    }

    expect(caught).toBeInstanceOf(Error);
    expect(isApiTimeoutError(caught)).toBe(false);
  });

  it("timeoutMs 为 0 表示不设上限", async () => {
    await expect(withRequestTimeout("/api/x", 0, undefined, async () => "ok")).resolves.toBe("ok");
  });
});
