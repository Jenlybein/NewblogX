import { beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, reactive, ref } from "vue";

import { useStudioArticle } from "~/composables/useStudioArticle";

// useStudioArticle 的用例。
//
// 这一块原本埋在 2000 行的写作页里、零覆盖。它承担的是"一篇文章当前是什么内容"，
// 其中最容易出错、又最不容易被发现的不是渲染，而是三件事：
//
//   1. **回填漏字段** —— 用户点开旧文章改，少回填一个字段，保存时就会把那个字段
//      悄悄清成零值。所以这里逐字段断言，而不是只看 title 对不对。
//   2. **回填覆盖用户输入** —— 回填只应发生一次；如果 watch 在取数 ref 抖动时
//      又跑一遍，用户刚敲的标题会被服务端旧值冲掉。幂等性单独钉一个用例。
//   3. **封面上传的失败路径** —— 上传失败时 pendingState.cover 必须复位，
//      否则发布按钮会永久卡在"上传中"（isPublishingLocked 读这一位）。

const getArticleDetailMock = vi.hoisted(() => vi.fn());
const getTagOptionsMock = vi.hoisted(() => vi.fn());
const getCategoryOptionsMock = vi.hoisted(() => vi.fn());
const createCategoryMock = vi.hoisted(() => vi.fn());
const uploadImageByTaskMock = vi.hoisted(() => vi.fn());
const buildPayloadMock = vi.hoisted(() => vi.fn());
const useAuthStoreMock = vi.hoisted(() => vi.fn());

vi.mock("~/services/article", () => ({ getArticleDetail: getArticleDetailMock }));
vi.mock("~/services/search", () => ({
  getTagOptions: getTagOptionsMock,
  getCategoryOptions: getCategoryOptionsMock,
  createCategory: createCategoryMock,
}));
vi.mock("~/services/image", () => ({ uploadImageByTask: uploadImageByTaskMock }));
vi.mock("~/utils/studioArticlePayload", () => ({ buildStudioArticlePayload: buildPayloadMock }));
vi.mock("~/stores/auth", () => ({ useAuthStore: useAuthStoreMock }));

/** 一篇"服务端返回的文章"，字段故意取非默认值，方便看出回填是否真的发生。 */
const articleFixture = {
  id: "100",
  title: "原标题",
  abstract: "原摘要",
  content: "原正文\n第二行",
  cover: "https://cdn.example.com/cover.png",
  cover_image_id: "img-1",
  comments_toggle: false,
  visibility_status: "user_hidden",
  category_id: "cat-1",
  tag_ids: ["tag-1", "tag-2"],
  publish_status: 3,
};

/**
 * settle：等取数落地 + watch 跑完。
 *
 * 为什么不能只 await 一次：useAsyncData 的 mock 是在微任务链里把结果写进 data 的，
 * 而回填发生在监听 data 的 watch 里（还要再等一次 nextTick 刷新）。
 * 宏任务 + 两次 nextTick 是最省事又稳定的组合。
 */
async function settle() {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await nextTick();
  await nextTick();
}

function makeMessage() {
  return { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() };
}

type HarnessOptions = {
  query?: Record<string, unknown>;
  profileId?: string;
  pending?: Partial<{ draft: boolean; publish: boolean; ai: boolean; cover: boolean }>;
  /** 读原图尺寸的替身：默认返回合规的 1920×1080（直接上传路径）。 */
  readSize?: (file: File) => Promise<{ width: number; height: number }>;
};

async function harness(options: HarnessOptions = {}) {
  const { query = {}, profileId = "u1" } = options;

  // route 必须是响应式对象：用例要靠"改 query"来模拟路由切换，
  // 普通对象不会触发 computed 重算，切回新建态那条用例就测不出来。
  const routeState = reactive<{ query: Record<string, unknown>; params: Record<string, unknown>; path: string }>({
    query: { ...query },
    params: {},
    path: "/studio/write",
  });
  vi.stubGlobal("__useRouteMock", () => routeState);
  useAuthStoreMock.mockReturnValue({ profileId });

  const message = makeMessage();
  const pendingState = { draft: false, publish: false, ai: false, cover: false, ...options.pending };
  const previewHeadings = ref<{ id: string; title: string; level: number }[]>([]);

  const api = await useStudioArticle({
    pendingState,
    message,
    previewHeadings,
    readImageSize: options.readSize ?? (async () => ({ width: 1920, height: 1080 })),
  });
  await settle();

  return { api, message, pendingState, previewHeadings, routeState };
}

/** 造一个假的 file input change 事件（happy-dom 里 File 可用，但不需要真实 DOM）。 */
function fileChangeEvent(file: File) {
  const input = { files: [file], value: "stale" } as unknown as HTMLInputElement;
  return { target: input } as unknown as Event;
}

function imageFile(name = "cover.png") {
  return new File(["x"], name, { type: "image/png" });
}

// 默认返回值统一放在这里，而不是 harness() 里。
//
// 为什么：harness() 内部就会触发取数（useAsyncData 的 mock 是"立刻发起"），
// 如果 harness 再设一次默认值，用例里先前配的 fixture / reject 会被悄悄覆盖，
// 表现为"我明明 mock 了却没生效"。这类时序坑很难查，所以默认值只在这里配一次。
beforeEach(() => {
  getArticleDetailMock.mockReset();
  getTagOptionsMock.mockReset();
  getCategoryOptionsMock.mockReset();
  createCategoryMock.mockReset();
  uploadImageByTaskMock.mockReset();
  buildPayloadMock.mockReset();
  useAuthStoreMock.mockReset();

  getTagOptionsMock.mockResolvedValue([
    { label: "Go", value: "tag-1" },
    { label: "Vue", value: "tag-2" },
  ]);
  getCategoryOptionsMock.mockResolvedValue([{ label: "后端", value: "cat-1" }]);
  createCategoryMock.mockResolvedValue({});
  getArticleDetailMock.mockResolvedValue(null);
  uploadImageByTaskMock.mockResolvedValue({ image_id: "img-9", url: "https://cdn.example.com/new.png" });
  buildPayloadMock.mockReturnValue({ title: "t", content: "c", status: 1 });
  useAuthStoreMock.mockReturnValue({ profileId: "u1" });
});

describe("useStudioArticle · 文章标识", () => {
  it("没有 id 时是新建态，按钮文案不带「更新」", async () => {
    const { api } = await harness();

    expect(api.editArticleId.value).toBe("");
    expect(api.isEditMode.value).toBe(false);
    expect(api.draftActionLabel.value).toBe("保存草稿");
    expect(api.publishActionLabel.value).toBe("确定并发布");
    expect(api.canOpenArticleScore.value).toBe(false);
  });

  it("?article_id= 决定编辑态，文案随之改变", async () => {
    const { api } = await harness({ query: { article_id: "100" } });

    expect(api.editArticleId.value).toBe("100");
    expect(api.isEditMode.value).toBe(true);
    expect(api.draftActionLabel.value).toBe("更新草稿");
    expect(api.publishActionLabel.value).toBe("更新并发布");
    expect(api.canOpenArticleScore.value).toBe(true);
  });

  it("?id= 作为兜底，并去掉首尾空白", async () => {
    const { api } = await harness({ query: { id: "  42  " } });

    expect(api.editArticleId.value).toBe("42");
    expect(api.isEditMode.value).toBe(true);
  });

  it("article_id 优先于 id", async () => {
    const { api } = await harness({ query: { article_id: "100", id: "42" } });

    expect(api.editArticleId.value).toBe("100");
  });

  it("非字符串 query（数组）不应把 id 读成数组", async () => {
    vi.stubGlobal("__useRouteMock", () => ({ query: { article_id: ["1", "2"] }, params: {}, path: "/" }));
    useAuthStoreMock.mockReturnValue({ profileId: "u1" });
    getTagOptionsMock.mockResolvedValue([]);
    getCategoryOptionsMock.mockResolvedValue([]);

    const api = await useStudioArticle({
      pendingState: { draft: false, publish: false, ai: false, cover: false },
      message: makeMessage(),
      previewHeadings: ref([]),
    });

    expect(api.editArticleId.value).toBe("");
  });
});

describe("useStudioArticle · 回填", () => {
  it("编辑态把服务端文章逐字段写回表单", async () => {
    getArticleDetailMock.mockResolvedValue({ ...articleFixture });
    const { api } = await harness({ query: { article_id: "100" } });

    expect(getArticleDetailMock).toHaveBeenCalledWith("100");
    expect(api.form.title).toBe("原标题");
    expect(api.form.abstract).toBe("原摘要");
    expect(api.form.content).toBe("原正文\n第二行");
    expect(api.form.cover_image_id).toBe("img-1");
    expect(api.coverPreviewUrl.value).toBe("https://cdn.example.com/cover.png");
    expect(api.form.comments_toggle).toBe(false);
    expect(api.form.visibility_status).toBe("user_hidden");
    expect(api.form.category_id).toBe("cat-1");
    expect(api.form.tag_ids).toEqual(["tag-1", "tag-2"]);
    expect(api.hydratedArticleId.value).toBe("100");
    expect(api.currentArticlePublishStatus.value).toBe(3);
  });

  it("回填只发生一次：取数 ref 再抖动也不会冲掉用户改的内容", async () => {
    getArticleDetailMock.mockResolvedValue({ ...articleFixture });
    const { api } = await harness({ query: { article_id: "100" } });

    api.form.title = "用户刚改的标题";
    // 模拟取数 ref 被换了一个新对象（同 id）
    (api.editingArticle as { value: unknown }).value = { ...articleFixture, title: "原标题" };
    await settle();

    expect(api.form.title).toBe("用户刚改的标题");
  });

  it("从编辑态切回新建态时清空旧文章残留（否则新建会带上上一篇的封面）", async () => {
    getArticleDetailMock.mockResolvedValue({ ...articleFixture });
    const { api, routeState } = await harness({ query: { article_id: "100" } });
    expect(api.hydratedArticleId.value).toBe("100");
    expect(api.form.cover_image_id).toBe("img-1");

    routeState.query = {};
    await settle();

    expect(api.editArticleId.value).toBe("");
    expect(api.hydratedArticleId.value).toBe("");
    expect(api.coverPreviewUrl.value).toBe("");
    expect(api.form.cover_image_id).toBeNull();
    expect(api.form.visibility_status).toBe("visible");
    expect(api.coverDirty.value).toBe(false);
  });

  it("visibility_status 是没见过的值时回落到 visible（不能透传脏值）", async () => {
    getArticleDetailMock.mockResolvedValue({ ...articleFixture, visibility_status: "whatever" });
    const { api } = await harness({ query: { article_id: "100" } });

    expect(api.form.visibility_status).toBe("visible");
  });

  it("tag_ids 不是数组时当空处理", async () => {
    getArticleDetailMock.mockResolvedValue({ ...articleFixture, tag_ids: "not-an-array" });
    const { api } = await harness({ query: { article_id: "100" } });

    expect(api.form.tag_ids).toEqual([]);
  });

  it("取数失败时提示错误，且不静默", async () => {
    getArticleDetailMock.mockRejectedValue(new Error("文章不存在"));
    const { api, message } = await harness({ query: { article_id: "100" } });

    expect(message.error).toHaveBeenCalledWith("文章不存在");
    expect(api.hydratedArticleId.value).toBe("");
  });

  it("分类选项只在登录后才请求（未登录不带上 userId）", async () => {
    await harness({ profileId: "" });

    expect(getCategoryOptionsMock).not.toHaveBeenCalled();
  });
});

describe("useStudioArticle · 派生状态", () => {
  it("contentStats 统计原始长度、去空白长度与行数", async () => {
    const { api } = await harness();
    api.form.content = "ab cd\n ef ";

    // "ab cd\n ef " → 原始 10 个字符，去掉 4 个空白后 6 个，\n 分成 2 行
    expect(api.contentStats.value.raw).toBe(10);
    expect(api.contentStats.value.pure).toBe(6);
    expect(api.contentStats.value.lines).toBe(2);
  });

  it("空正文时行数按 1 算（UI 上显示「1 行」而不是 0）", async () => {
    const { api } = await harness();

    expect(api.contentStats.value.lines).toBe(1);
  });

  it("tocItems 由预览区上报的锚点映射而来", async () => {
    const { api, previewHeadings } = await harness();
    previewHeadings.value = [
      { id: "h1", title: "一级标题", level: 2 },
      { id: "h2", title: "二级标题", level: 3 },
    ];
    await nextTick();

    expect(api.tocItems.value).toEqual([
      { id: "h1", label: "一级标题", level: 2 },
      { id: "h2", label: "二级标题", level: 3 },
    ]);
  });

  it("canSubmit 要求标题与正文都非空", async () => {
    const { api } = await harness();
    expect(api.canSubmit.value).toBe(false);

    api.form.title = "标题";
    expect(api.canSubmit.value).toBe(false);

    api.form.content = "正文";
    expect(api.canSubmit.value).toBe(true);
  });

  it("isPublishingLocked 被任意进行中标记锁住", async () => {
    const { api } = await harness();
    expect(api.isPublishingLocked.value).toBe(false);

    const { api: api2 } = await harness({ pending: { ai: true } });
    expect(api2.isPublishingLocked.value).toBe(true);
  });

  it("buildArticlePayload 把状态、编辑态、封面脏标记一起传下去", async () => {
    const { api } = await harness({ query: { article_id: "100" } });
    api.coverDirty.value = true;

    api.buildArticlePayload(2);

    expect(buildPayloadMock).toHaveBeenCalledWith(
      api.form,
      expect.objectContaining({ status: 2, isEditMode: true, coverDirty: true }),
    );
  });
});

describe("useStudioArticle · 封面", () => {
  it("上传进行中时不再触发文件选择（避免并发上传）", async () => {
    const { api } = await harness({ pending: { cover: true } });
    const click = vi.fn();
    api.coverFileInputRef.value = { click } as never;

    api.openCoverFilePicker();

    expect(click).not.toHaveBeenCalled();
  });

  it("非图片文件直接拦下，不发上传请求", async () => {
    const { api, message } = await harness();

    await api.handleCoverFileChange(fileChangeEvent(new File(["x"], "a.txt", { type: "text/plain" })));

    expect(message.warning).toHaveBeenCalledWith("请选择图片文件作为封面。");
    expect(uploadImageByTaskMock).not.toHaveBeenCalled();
  });

  it("上传成功写入 image_id 与预览地址，并标记封面已改动", async () => {
    const { api, message, pendingState } = await harness();

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));

    expect(uploadImageByTaskMock).toHaveBeenCalled();
    expect(api.form.cover_image_id).toBe("img-9");
    expect(api.coverPreviewUrl.value).toBe("https://cdn.example.com/new.png");
    expect(api.coverDirty.value).toBe(true);
    expect(api.coverUploadStage.value).toBe("封面上传完成");
    expect(message.success).toHaveBeenCalledWith("封面上传成功");
    expect(pendingState.cover).toBe(false);
  });

  it("上传失败要复位进行中标记，否则发布按钮会永久卡住", async () => {
    uploadImageByTaskMock.mockRejectedValue(new Error("网络断开"));
    const { api, message, pendingState } = await harness();

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));

    expect(message.error).toHaveBeenCalledWith("网络断开");
    expect(api.coverUploadStage.value).toBe("");
    expect(pendingState.cover).toBe(false);
    expect(api.form.cover_image_id).toBeNull();
  });

  it("上传成功但没返回 image_id 时按失败处理（不能写入空 id）", async () => {
    uploadImageByTaskMock.mockResolvedValue({ image_id: "", url: "" });
    const { api, message, pendingState } = await harness();

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));

    expect(message.error).toHaveBeenCalledWith("上传成功但未返回可用图片标识");
    expect(api.form.cover_image_id).toBeNull();
    expect(pendingState.cover).toBe(false);
  });

  it("上传过程会播报阶段文案", async () => {
    uploadImageByTaskMock.mockImplementation(async (_file: File, options: { onStage: (s: never) => void }) => {
      options.onStage("hashing" as never);
      options.onStage("uploading_to_qiniu" as never);
      return { image_id: "img-9", url: "https://cdn.example.com/new.png" };
    });
    const { api } = await harness();

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));

    expect(api.coverUploadStage.value).toBe("封面上传完成");
    // 直接验证映射表本身
    api.mapCoverUploadStage("creating_task");
    expect(api.coverUploadStage.value).toBe("正在创建上传任务…");
    api.mapCoverUploadStage("polling_status");
    expect(api.coverUploadStage.value).toBe("正在确认图片状态…");
  });

  it("清除封面会清空预览并把封面标脏（否则服务端还留着旧封面）", async () => {
    const { api } = await harness({ query: { article_id: "100" } });
    getArticleDetailMock.mockResolvedValue({ ...articleFixture });
    // 先回填出封面
    (api.editingArticle as { value: unknown }).value = { ...articleFixture };
    await settle();
    expect(api.form.cover_image_id).toBe("img-1");

    api.coverFileInputRef.value = { value: "stale" } as never;
    api.clearCoverSelection();

    expect(api.form.cover_image_id).toBeNull();
    expect(api.coverPreviewUrl.value).toBe("");
    expect(api.coverUploadStage.value).toBe("");
    expect(api.coverDirty.value).toBe(true);
  });

  it("没有文件时直接返回，不做任何提示", async () => {
    const { api, message } = await harness();
    const input = { files: [], value: "" } as unknown as HTMLInputElement;

    await api.handleCoverFileChange({ target: input } as unknown as Event);

    expect(message.warning).not.toHaveBeenCalled();
    expect(message.error).not.toHaveBeenCalled();
    expect(uploadImageByTaskMock).not.toHaveBeenCalled();
  });
});

describe("useStudioArticle · 内联新建分类", () => {
  it("登录状态由当前用户决定", async () => {
    const { api } = await harness();
    expect(api.isLoggedIn.value).toBe(true);

    const { api: guestApi } = await harness({ profileId: "" });
    expect(guestApi.isLoggedIn.value).toBe(false);
  });

  it("创建成功后重新载入下拉，并把新分类自动选中", async () => {
    const { api, message } = await harness();
    getCategoryOptionsMock.mockResolvedValueOnce([
      { label: "后端", value: "cat-1" },
      { label: "新分类", value: "cat-9" },
    ]);

    const ok = await api.createCategoryInline("  新分类  ");

    expect(ok).toBe(true);
    expect(createCategoryMock).toHaveBeenCalledWith("新分类");
    expect(api.categoryOptions.value).toEqual([
      { label: "后端", value: "cat-1" },
      { label: "新分类", value: "cat-9" },
    ]);
    expect(api.form.category_id).toBe("cat-9");
    expect(message.success).toHaveBeenCalledWith("已新建分类「新分类」");
  });

  it("重名时展示后端返回的可读业务文案，且不选中任何分类", async () => {
    const { api, message } = await harness();
    createCategoryMock.mockRejectedValue(new Error("名称重复"));

    const ok = await api.createCategoryInline("新分类");

    expect(ok).toBe(false);
    expect(message.error).toHaveBeenCalledWith("创建分类失败：名称重复");
    expect(api.form.category_id).toBeNull();
    expect(message.success).not.toHaveBeenCalled();
  });

  it("空标题与纯空白标题在发请求前就被拒绝", async () => {
    const { api, message } = await harness();

    for (const raw of ["", "   ", "\t\n"]) {
      const ok = await api.createCategoryInline(raw);
      expect(ok).toBe(false);
    }

    expect(createCategoryMock).not.toHaveBeenCalled();
    expect(message.warning).toHaveBeenCalledWith("请输入分类名称");
  });

  it("未登录时入口不可用，不发创建请求", async () => {
    const { api, message } = await harness({ profileId: "" });

    const ok = await api.createCategoryInline("新分类");

    expect(ok).toBe(false);
    expect(createCategoryMock).not.toHaveBeenCalled();
    expect(message.warning).toHaveBeenCalledWith("请先登录后再新建分类");
  });

  it("创建成功但下拉里找不到新分类时给出可见提示，不静默失败", async () => {
    const { api, message } = await harness();
    getCategoryOptionsMock.mockResolvedValueOnce([{ label: "后端", value: "cat-1" }]);

    const ok = await api.createCategoryInline("新分类");

    expect(ok).toBe(false);
    expect(message.warning).toHaveBeenCalledWith("分类已创建，但未能载入新分类，请稍后重试。");
    expect(api.form.category_id).toBeNull();
    expect(message.success).not.toHaveBeenCalled();
  });

  it("创建成功但重新载入下拉失败时区分于「创建失败」", async () => {
    const { api, message } = await harness();
    getCategoryOptionsMock.mockRejectedValueOnce(new Error("网络断开"));

    const ok = await api.createCategoryInline("新分类");

    expect(ok).toBe(false);
    expect(message.error).toHaveBeenCalledWith("分类已创建，但分类列表加载失败，请手动刷新后选择。");
    expect(message.success).not.toHaveBeenCalled();
  });
});

describe("useStudioArticle · 封面 16:9 锁定与取景", () => {
  it("合规图直接上传，不打开取景窗", async () => {
    const { api } = await harness({ readSize: async () => ({ width: 1920, height: 1080 }) });

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));

    expect(uploadImageByTaskMock).toHaveBeenCalledTimes(1);
    expect(api.coverFramingOpen.value).toBe(false);
    expect(api.coverImageSize.value).toEqual({ width: 1920, height: 1080 });
    expect(api.coverRoute.value).toBe("direct");
    expect(api.coverBadge.value?.text).toBe("16:9 合规");
  });

  it("比例不合规先打开取景窗，不发起上传（上传要等确认）", async () => {
    const { api, pendingState } = await harness({ readSize: async () => ({ width: 1200, height: 800 }) });

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));

    expect(uploadImageByTaskMock).not.toHaveBeenCalled();
    expect(api.coverFramingOpen.value).toBe(true);
    expect(api.coverRoute.value).toBe("frame");
    expect(api.coverBadge.value?.text).toBe("比例不合规，需取景");
    // 取景阶段不能锁住发布按钮：还没有任何上传在途。
    expect(pendingState.cover).toBe(false);
    expect(api.form.cover_image_id).toBeNull();
  });

  it("确认取景后才上传，并标记已取景", async () => {
    const { api, message } = await harness({ readSize: async () => ({ width: 1200, height: 800 }) });

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));
    await api.confirmCoverFraming({ focus_x: 0.3, focus_y: 0.4, zoom: 1.2 });

    expect(uploadImageByTaskMock).toHaveBeenCalledTimes(1);
    expect(api.form.cover_image_id).toBe("img-9");
    expect(api.coverPreviewUrl.value).toBe("https://cdn.example.com/new.png");
    expect(api.coverDirty.value).toBe(true);
    expect(api.coverFramingOpen.value).toBe(false);
    expect(api.coverFramed.value).toBe(true);
    expect(api.coverBadge.value?.text).toBe("已取景");
    expect(message.success).toHaveBeenCalledWith("封面上传成功");
  });

  it("取景参数只用于预览，不写进文章载荷（本票不落库）", async () => {
    const { api } = await harness({ readSize: async () => ({ width: 1200, height: 800 }) });

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));
    await api.confirmCoverFraming({ focus_x: 0.3, focus_y: 0.4, zoom: 1.2 });
    api.buildArticlePayload(2);

    // 载荷仍是既有契约的字段集合：没有任何 crop / framing 字段。
    const payload = buildPayloadMock.mock.calls.at(-1)?.[0] as Record<string, unknown>;
    expect(Object.keys(payload)).not.toContain("crop");
    expect(Object.keys(payload)).not.toContain("framing");
    expect(buildPayloadMock.mock.calls.at(-1)?.[1]).toEqual({
      status: 2,
      isEditMode: false,
      coverDirty: true,
    });
  });

  it("取消取景＝本次不上传：没有图片记录，也没有预览", async () => {
    const { api, message } = await harness({ readSize: async () => ({ width: 1200, height: 800 }) });

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));
    api.cancelCoverFraming();

    expect(uploadImageByTaskMock).not.toHaveBeenCalled();
    expect(api.coverFramingOpen.value).toBe(false);
    expect(api.form.cover_image_id).toBeNull();
    expect(api.coverPreviewUrl.value).toBe("");
    expect(api.coverFramed.value).toBe(false);
    expect(message.info).toHaveBeenCalledWith(expect.stringContaining("已取消取景"));
  });

  it("读不到原图尺寸时按失败处理：不弹取景窗、不上传", async () => {
    const { api, message } = await harness({
      readSize: async () => {
        throw new Error("图片尺寸读取失败");
      },
    });

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));

    expect(uploadImageByTaskMock).not.toHaveBeenCalled();
    expect(api.coverFramingOpen.value).toBe(false);
    expect(api.form.cover_image_id).toBeNull();
    expect(message.warning).toHaveBeenCalledWith("读不到这张图片的尺寸，请换一张图片作为封面。");
  });

  it("上传失败把错误留在面板上，重新选择时清掉", async () => {
    uploadImageByTaskMock.mockRejectedValueOnce(new Error("网络断开"));
    const { api } = await harness({ readSize: async () => ({ width: 1920, height: 1080 }) });

    await api.handleCoverFileChange(fileChangeEvent(imageFile()));
    expect(api.coverError.value).toBe("网络断开");

    uploadImageByTaskMock.mockResolvedValue({ image_id: "img-9", url: "https://cdn.example.com/new.png" });
    await api.handleCoverFileChange(fileChangeEvent(imageFile()));
    expect(api.coverError.value).toBe("");
  });

  it("回填的旧封面没有原图尺寸：不显示合规徽标，也不假装取过景", async () => {
    const { api } = await harness({ query: { article_id: "100" } });
    getArticleDetailMock.mockResolvedValue({ ...articleFixture });
    (api.editingArticle as { value: unknown }).value = { ...articleFixture };
    await settle();

    expect(api.coverImageSize.value).toBeNull();
    expect(api.coverRoute.value).toBe("");
    expect(api.coverBadge.value).toBeNull();
    expect(api.coverFramed.value).toBe(false);
  });
});
