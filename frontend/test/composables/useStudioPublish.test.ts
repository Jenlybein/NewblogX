import { beforeEach, describe, expect, it, vi } from "vitest";

import { useStudioPublish } from "~/composables/useStudioPublish";

// useStudioPublish 的 6 条分支。
//
// 这个模块是写作页唯一会产生写操作的地方：新建/编辑 × 草稿/发布 = 4 条主路径，
// 加上"发布后直接可见 / 需要审核"两种结果文案。它们看起来一样但提示各不相同 ——
// 用户是照着提示去操作下一步的（"已提交审核"和"已发布"意味着完全不同的等待行为）。
//
// 以前这些分支埋在 2000 行的页面里，没有任何测试；现在抽出来了，逐条钉住。

const createArticleMock = vi.hoisted(() => vi.fn());
const updateArticleMock = vi.hoisted(() => vi.fn());

vi.mock("~/services/article", () => ({
  createArticle: createArticleMock,
  updateArticle: updateArticleMock,
}));

type Harness = {
  publish: ReturnType<typeof useStudioPublish>;
  message: { success: ReturnType<typeof vi.fn>; warning: ReturnType<typeof vi.fn>; error: ReturnType<typeof vi.fn> };
  push: ReturnType<typeof vi.fn>;
  pendingState: { draft: boolean; publish: boolean; ai: boolean; cover: boolean };
  publishVisible: { value: boolean };
};

function harness(overrides: {
  isEdit?: boolean;
  articleId?: string;
  canSubmit?: boolean;
  visibility?: "visible" | "user_hidden";
  coverUploading?: boolean;
} = {}): Harness {
  const message = { success: vi.fn(), warning: vi.fn(), error: vi.fn() };
  const push = vi.fn(async () => undefined);
  const pendingState = { draft: false, publish: false, ai: false, cover: overrides.coverUploading ?? false };
  const publishVisible = { value: true };

  const publish = useStudioPublish({
    canSubmit: { value: overrides.canSubmit ?? true },
    pendingState,
    isEditMode: { value: overrides.isEdit ?? false },
    editArticleId: { value: overrides.articleId ?? "" },
    publishVisible,
    visibilityStatus: () => overrides.visibility ?? "visible",
    buildPayload: (status) => ({ title: "t", content: "c", status }) as never,
    message,
    router: { push },
  });

  return { publish, message, push, pendingState, publishVisible };
}

beforeEach(() => {
  createArticleMock.mockReset();
  updateArticleMock.mockReset();
});

describe("useStudioPublish", () => {
  it("标题或正文为空时直接拦下，不发请求", async () => {
    const h = harness({ canSubmit: false });

    await h.publish.submitArticle(1);

    expect(h.message.warning).toHaveBeenCalledWith("标题和正文是必填项。");
    expect(createArticleMock).not.toHaveBeenCalled();
    expect(updateArticleMock).not.toHaveBeenCalled();
  });

  it("封面仍在上传时不允许提交（否则会提交到没有封面的状态）", async () => {
    const h = harness({ coverUploading: true });

    await h.publish.submitArticle(2);

    expect(h.message.warning).toHaveBeenCalledWith("封面仍在上传中，请稍后再提交。");
    expect(createArticleMock).not.toHaveBeenCalled();
  });

  it("新建 + 存草稿：提示「草稿已保存」并跳到文章页", async () => {
    createArticleMock.mockResolvedValue({ id: "42", publish_status: 1 });
    const h = harness();

    await h.publish.submitArticle(1);

    expect(createArticleMock).toHaveBeenCalledWith(expect.objectContaining({ status: 1 }));
    expect(h.message.success).toHaveBeenCalledWith("草稿已保存");
    expect(h.push).toHaveBeenCalledWith("/article/42");
    expect(h.publishVisible.value).toBe(false);
  });

  it("新建 + 发布：按返回的 publish_status 给三种不同提示", async () => {
    const cases = [
      { status: 3, want: "文章已发布成功" },
      { status: 2, want: "文章已提交审核，审核通过后会公开展示" },
      { status: 0, want: "文章已提交发布" },
    ];

    for (const c of cases) {
      createArticleMock.mockResolvedValue({ id: "7", publish_status: c.status });
      const h = harness();
      await h.publish.submitArticle(2);
      expect(h.message.success).toHaveBeenCalledWith(c.want);
    }
  });

  it("编辑态 + 存草稿：走 update 而不是 create", async () => {
    updateArticleMock.mockResolvedValue(undefined);
    const h = harness({ isEdit: true, articleId: "9" });

    await h.publish.submitArticle(1);

    expect(updateArticleMock).toHaveBeenCalledWith("9", expect.objectContaining({ status: 1 }));
    expect(createArticleMock).not.toHaveBeenCalled();
    expect(h.message.success).toHaveBeenCalledWith("草稿已更新");
    expect(h.push).toHaveBeenCalledWith("/article/9");
  });

  it("编辑态 + 发布：可见性只自己时提示带括号说明", async () => {
    updateArticleMock.mockResolvedValue(undefined);
    const h = harness({ isEdit: true, articleId: "9", visibility: "user_hidden" });

    await h.publish.submitArticle(2);

    expect(h.message.success).toHaveBeenCalledWith("文章已提交发布（当前为仅自己可见）");
  });

  it("请求失败时提示错误，并且不允许 loading 卡住", async () => {
    createArticleMock.mockRejectedValue(new Error("网络断开"));
    const h = harness();

    await h.publish.submitArticle(1);

    expect(h.message.error).toHaveBeenCalledWith("网络断开");
    expect(h.pendingState.draft).toBe(false);
    expect(h.pendingState.publish).toBe(false);
  });
});
