import { beforeEach, describe, expect, it, vi } from "vitest";
import { computeQetag, uploadImageByTask } from "~/services/image";

const requestMock = vi.fn();
const fetchMock = vi.fn();

describe("image service", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
    requestMock.mockReset();
    fetchMock.mockReset();
  });

  it("returns the backend url immediately when the upload task can skip qiniu upload", async () => {
    requestMock.mockResolvedValue({
      skip_upload: true,
      url: "https://image.example.com/avatar.png",
      hash: "etag-1",
      image_id: "img-1",
      status: "ready",
    });

    const result = await uploadImageByTask(new File(["avatar"], "avatar.png", { type: "image/png" }));

    expect(requestMock).toHaveBeenCalledWith("/api/images/upload-tasks", expect.objectContaining({
      method: "POST",
      body: expect.objectContaining({
        file_name: "avatar.png",
        mime_type: "image/png",
      }),
    }));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(result).toEqual({
      url: "https://image.example.com/avatar.png",
      hash: "etag-1",
      image_id: "img-1",
      status: "ready",
      skip_upload: true,
    });
  });

  it("creates a stable url-safe qetag for the same file content", async () => {
    const file = new File(["avatar"], "avatar.png", { type: "image/png" });

    const first = await computeQetag(file);
    const second = await computeQetag(file);

    expect(first).toBe(second);
    expect(first).toMatch(/^[A-Za-z0-9\-_]+$/);
  });

  it("falls back when WebCrypto subtle digest is unavailable", async () => {
    const originalCrypto = globalThis.crypto;
    vi.stubGlobal("crypto", {});

    try {
      const file = new File(["avatar"], "avatar.png", { type: "image/png" });
      const first = await computeQetag(file);
      const second = await computeQetag(file);

      expect(first).toBe(second);
      expect(first).toMatch(/^[A-Za-z0-9\-_]+$/);
    } finally {
      vi.stubGlobal("crypto", originalCrypto);
    }
  });

  it("uploads to the region-specific qiniu host and polls until the asset is ready", async () => {
    requestMock
      .mockResolvedValueOnce({
        skip_upload: false,
        upload_id: "upload-1",
        upload_token: "token-1",
        object_key: "avatars/u1.png",
        region: "z1",
      })
      .mockResolvedValueOnce({
        status: "processing",
      })
      .mockResolvedValueOnce({
        status: "ready",
        url: "https://image.example.com/avatars/u1.png",
        hash: "etag-2",
        image_id: "img-2",
        upload_id: "upload-1",
      });

    fetchMock.mockResolvedValue({
      ok: true,
      status: 200,
    });

    const result = await uploadImageByTask(
      new File(["avatar"], "avatar.png", { type: "image/png" }),
      {
        pollIntervalMs: 0,
        pollTimeoutMs: 100,
      },
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "https://upload-z1.qiniup.com",
      expect.objectContaining({
        method: "POST",
        body: expect.any(FormData),
      }),
    );
    expect(requestMock).toHaveBeenNthCalledWith(2, "/api/images/upload-tasks/upload-1");
    expect(requestMock).toHaveBeenNthCalledWith(3, "/api/images/upload-tasks/upload-1");
    expect(result).toEqual({
      url: "https://image.example.com/avatars/u1.png",
      hash: "etag-2",
      image_id: "img-2",
      upload_id: "upload-1",
      status: "ready",
      skip_upload: false,
    });
  });

  it("fails fast when the backend task misses required qiniu upload fields", async () => {
    requestMock.mockResolvedValue({
      skip_upload: false,
      upload_id: "upload-1",
      upload_token: "token-1",
    });

    await expect(
      uploadImageByTask(new File(["avatar"], "avatar.png", { type: "image/png" })),
    ).rejects.toThrow("上传任务缺少必要字段（upload_id/upload_token/object_key）");

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("throws when a skip-upload task does not provide a final image url", async () => {
    requestMock.mockResolvedValue({
      skip_upload: true,
      hash: "etag-3",
      image_id: "img-3",
      status: "ready",
    });

    await expect(
      uploadImageByTask(new File(["avatar"], "avatar.png", { type: "image/png" })),
    ).rejects.toThrow("上传任务命中秒传，但未返回图片 URL");
  });

  it("surfaces backend polling failures before returning a broken ready state", async () => {
    requestMock
      .mockResolvedValueOnce({
        skip_upload: false,
        upload_id: "upload-2",
        upload_token: "token-2",
        object_key: "avatars/u2.png",
      })
      .mockResolvedValueOnce({
        status: "failed",
        error_msg: "服务端处理失败",
      });

    fetchMock.mockResolvedValue({
      ok: true,
      status: 200,
    });

    await expect(
      uploadImageByTask(new File(["avatar"], "avatar.png", { type: "image/png" }), {
        pollIntervalMs: 0,
        pollTimeoutMs: 100,
      }),
    ).rejects.toThrow("服务端处理失败");

    expect(fetchMock).toHaveBeenCalledWith(
      "https://upload.qiniup.com",
      expect.objectContaining({
        method: "POST",
      }),
    );
  });
});
