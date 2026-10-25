import { describe, expect, it } from "vitest";

import { buildStudioArticlePayload, type StudioArticleForm } from "~/utils/studioArticlePayload";

// 提交载荷的三个"看起来无所谓、实际会改线上语义"的规则。
// 以前它们埋在页面闭包里，没有任何测试；抽成纯函数后逐条钉住。

function form(overrides: Partial<StudioArticleForm> = {}): StudioArticleForm {
  return {
    title: "  标题  ",
    abstract: "",
    content: "正文",
    category_id: null,
    tag_ids: [],
    comments_toggle: true,
    visibility_status: "visible",
    cover_image_id: null,
    ...overrides,
  };
}

describe("buildStudioArticlePayload", () => {
  it("标题去首尾空格，正文原样保留", () => {
    const payload = buildStudioArticlePayload(form(), { status: 1, isEditMode: false, coverDirty: false });

    expect(payload.title).toBe("标题");
    expect(payload.content).toBe("正文");
  });

  it("空摘要要变成 undefined（不是空串）", () => {
    const payload = buildStudioArticlePayload(form({ abstract: "   " }), { status: 1, isEditMode: false, coverDirty: false });

    // 后端对 "" 的理解是"把摘要写成空"，对 undefined 是"不改这列"
    expect(payload.abstract).toBeUndefined();
    expect("abstract" in payload).toBe(true);
  });

  it("有摘要时去空格后带上", () => {
    const payload = buildStudioArticlePayload(form({ abstract: " 摘要 " }), { status: 2, isEditMode: false, coverDirty: false });

    expect(payload.abstract).toBe("摘要");
  });

  it("空分类变成 null（而不是空串）", () => {
    const payload = buildStudioArticlePayload(form({ category_id: "" }), { status: 1, isEditMode: false, coverDirty: false });

    expect(payload.category_id).toBeNull();
  });

  it("封面：有 id 就带上", () => {
    const payload = buildStudioArticlePayload(form({ cover_image_id: "img-1" }), {
      status: 2,
      isEditMode: true,
      coverDirty: true,
    });

    expect(payload.cover_image_id).toBe("img-1");
  });

  it("封面：编辑态且主动清过封面时显式传 null（表示「删掉封面」）", () => {
    const payload = buildStudioArticlePayload(form({ cover_image_id: null }), {
      status: 1,
      isEditMode: true,
      coverDirty: true,
    });

    // 漏了这条，"移除封面"会静默失效 —— 后端收到"没这个字段"就当成"不动"
    expect(payload.cover_image_id).toBeNull();
    expect("cover_image_id" in payload).toBe(true);
  });

  it("封面：新建时没有封面就不传这个字段（表示「本来就没有」）", () => {
    const payload = buildStudioArticlePayload(form({ cover_image_id: null }), {
      status: 1,
      isEditMode: false,
      coverDirty: false,
    });

    expect("cover_image_id" in payload).toBe(false);
  });

  it("编辑态但没动过封面时也不传（避免把已有封面误删）", () => {
    const payload = buildStudioArticlePayload(form({ cover_image_id: null }), {
      status: 2,
      isEditMode: true,
      coverDirty: false,
    });

    expect("cover_image_id" in payload).toBe(false);
  });

  it("status 原样透传（1=草稿 2=发布）", () => {
    expect(buildStudioArticlePayload(form(), { status: 1, isEditMode: false, coverDirty: false }).status).toBe(1);
    expect(buildStudioArticlePayload(form(), { status: 2, isEditMode: false, coverDirty: false }).status).toBe(2);
  });
});
