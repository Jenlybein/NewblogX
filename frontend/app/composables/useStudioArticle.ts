import { useAsyncData, useRoute } from "#imports";
import { computed, reactive, ref, watch, type Ref } from "vue";

import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { getArticleDetail } from "~/services/article";
import { buildStudioArticlePayload } from "~/utils/studioArticlePayload";
import { uploadImageByTask } from "~/services/image";
import { getCategoryOptions, getTagOptions, createCategory } from "~/services/search";
import { useAuthStore } from "~/stores/auth";
import { useImageCropUpload, type ImageFramingRequest } from "~/composables/useImageCropUpload";
import {
  isImageFile,
  measureImage,
  shapeForUpload,
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_RATIO_TEXT,
  type ShapeFramingState,
  type ShapedUploadFile,
  type ShapeImageTarget,
} from "~/utils/imageShaping";
import {
  categoryTitleError,
  findCategoryOptionByTitle,
  normalizeCategoryTitle,
} from "~/utils/studioCategory";
import { describeCoverBadge, type CoverBadge } from "~/utils/studioCover";
import { errorMessage } from "~/utils/errorMessage";

// 写作页的"文章数据域"：表单内容、数据加载、编辑态回填、封面上传、提交载荷。
//
// 从 pages/studio/write.vue 抽出来（M6 ⑧）。判据不是"代码多"，
// 而是这组东西**共同描述同一件事**：一篇文章当前是什么内容。
// 它们被 hydate（回填）、封面、发布三个流程共用，所以在页面里
// 谁也离不开谁，只能一起搬。
//
// 抽出来之后有三处直接收益：
//   1. "回填时漏了一个字段"这类问题有了明确位置 —— 只有 hydrate 一处写 form；
//   2. buildArticlePayload 是纯函数，可以直接测"给定 form 产出什么载荷"；
//   3. 页面不再需要知道 tag/category 从哪个接口来、封面怎么上传。
//
// 状态由本模块持有；跨域的"进行中"标记（pendingState）由页面持有并注入，
// 因为发布、AI、封面三处都要读它。


/** Notifier 是本模块需要的提示能力（只声明用到的四个方法）。 */
export type Notifier = {
  success: (msg: string) => void;
  warning: (msg: string) => void;
  error: (msg: string) => void;
  info: (msg: string) => void;
};

/** PendingState 是各流程的"进行中"标记，由页面持有。 */
export type PendingState = {
  draft: boolean;
  publish: boolean;
  ai: boolean;
  cover: boolean;
};

/**
 * 上传前定形器（可注入）：默认把选中的图定形成目标比例（16:9 / 1600×900）。
 * `framing` 存在时按作者在裁剪窗里选定的区域取景（票 215），缺省为自动居中。
 * 单元测试注入替身即可驱动"定形成功 / 定形失败"两条路径，不需要真实画布（ADR-0016）。
 */
export type CoverShaper = (file: File, framing?: ShapeFramingState) => Promise<ShapedUploadFile>;

/** 量原图尺寸（比例判定用）；默认走浏览器解码，可注入替身。 */
export type CoverMeasurer = (file: File) => Promise<ShapeImageTarget>;

function defaultShapeCover(file: File, framing?: ShapeFramingState): Promise<ShapedUploadFile> {
  const options = framing ? { ...COVER_SHAPE_OPTIONS, framing } : COVER_SHAPE_OPTIONS;
  return shapeForUpload(file, options);
}

function defaultMeasureCover(file: File): Promise<ShapeImageTarget> {
  return measureImage(file);
}

export type StudioArticleDeps = {
  /** 进行中标记（封面流程要读写 cover）。 */
  pendingState: PendingState;
  /** 提示。 */
  message: Notifier;
  /** 预览区上报的标题锚点（目录与统计要用）。 */
  previewHeadings: Ref<ArticleHeadingAnchor[]>;
  /**
   * 上传前定形（可选注入）：默认把选中的图定形成 16:9（1600×900）。
   * 原图不进入上传链路，页面只上传定形产物。
   */
  shapeCover?: CoverShaper;
  /** 量原图尺寸（可选注入）：默认浏览器解码；比例合规才不弹裁剪窗。 */
  measureCover?: CoverMeasurer;
};

/**
 * useStudioArticle 提供文章内容与它的加载/回填/封面能力。
 *
 * "当前编辑的是哪篇文章"由 URL 决定（`?article_id=` 或 `?id=`），
 * 所以 editArticleId 与数据加载都在本模块内 —— 页面只关心
 * "有没有 articleId"这一个派生结果。
 */
export async function useStudioArticle(deps: StudioArticleDeps) {
  const {
    pendingState,
    message,
    previewHeadings,
    shapeCover = defaultShapeCover,
    measureCover = defaultMeasureCover,
  } = deps;
  const route = useRoute();
  const authStore = useAuthStore();

  const currentUserId = computed(() => authStore.profileId || "");
  const isLoggedIn = computed(() => Boolean(currentUserId.value));

  const form = reactive({
    title: "",
    abstract: "",
    content: "",
    category_id: null as string | null,
    tag_ids: [] as string[],
    cover_image_id: null as string | null,
    comments_toggle: true,
    visibility_status: "visible" as "visible" | "user_hidden",
  });

  /**
   * 当前文章是否仍在"已发布、等内容检查通过才对读者可见"。
   *
   * 票 302 新规则引入 pending_review：作者拿到"已发布"回执，但读者还看不到。
   * 表单的 visibility_status 只有两档（写接口不收 pending_review），所以单独用
   * 一个只读标记承载它 —— 回填时归一成 visible 不会误导，页面据此给一句提示。
   */
  const pendingReview = ref(false);

  const coverFileInputRef = ref<HTMLInputElement | null>(null);

  const coverPreviewUrl = ref("");
  const coverUploadStage = ref("");
  /** 面板里留住的错误文案（上传失败不再只靠一闪而过的 toast）。 */
  const coverError = ref("");

  /** 本次选中的**定形产物**（票 200：原图不上传）。 */
  const coverPendingFile = ref<File | null>(null);
  /** 定形产物的像素尺寸（16:9、1600×900），用于面板上的尺寸读数与徽标。 */
  const coverImageSize = ref<ShapeImageTarget | null>(null);
  /** 定形产物的本地预览地址（上传在途时用，所见即即将上传的那张图）。 */
  const coverLocalPreviewUrl = ref("");

  const coverDirty = ref(false);

  /**
   * 裁剪窗请求（票 215）：非 null 表示"图片比例不合规，等作者选区域"。
   * 这期间没有定形产物、没有上传，现有封面（form.cover_image_id / coverPreviewUrl）保持不变。
   */
  const coverCropRequest = ref<ImageFramingRequest | null>(null);
  /** 待裁剪原图的本地预览地址（弹窗里可拖动的那张图）；关闭/确认后释放。 */
  const coverCropPreviewUrl = ref("");

  /** 本地预览地址：环境没有 object URL 时留空，不抛给页面。 */
  function createPreviewUrl(blob: Blob): string {
    try {
      return typeof URL.createObjectURL === "function" ? URL.createObjectURL(blob) : "";
    } catch {
      return "";
    }
  }

  function revokePreviewUrl(url: string) {
    if (!url) return;
    try {
      if (typeof URL.revokeObjectURL === "function") URL.revokeObjectURL(url);
    } catch {
      // 已经释放过或环境不支持：忽略。
    }
  }

  /** 关掉裁剪窗并释放原图预览（确认/取消/换图/切文章都要走这里）。 */
  function resetCoverCrop() {
    const url = coverCropPreviewUrl.value;
    coverCropPreviewUrl.value = "";
    coverCropRequest.value = null;
    revokePreviewUrl(url);
  }

  /**
   * 封面选图后的唯一顺序（票 215）：量尺寸 → 合规直接定形上传；不合规只打开裁剪窗。
   * 五个入口共用这一份编排（`useImageCropUpload`），页面不再各写一段。
   */
  const coverCropFlow = useImageCropUpload({
    options: COVER_SHAPE_OPTIONS,
    measure: (file) => measureCover(file),
    shape: (file, options) =>
      options.framing ? shapeCover(file, options.framing) : shapeCover(file),
    requestFraming: (request) => {
      coverCropRequest.value = request;
      coverCropPreviewUrl.value = createPreviewUrl(request.file);
      coverError.value = "";
      coverUploadStage.value = `图片比例不符合 ${COVER_SHAPE_RATIO_TEXT}，请选择要露出的区域`;
    },
    upload: async (shaped) => {
      // 预览与上传都只认定形产物（合规定形或裁剪确认后的产物）。
      coverPendingFile.value = shaped.file;
      coverImageSize.value = shaped.size;
      coverLocalPreviewUrl.value = shaped.previewUrl;
      coverUploadStage.value = `已定形成 ${shaped.size.width} × ${shaped.size.height}，正在上传封面…`;
      await uploadSelectedCover(shaped.file);
    },
  });

  /** 选过图（拿到了定形产物）才显示徽标；回填的旧封面没有尺寸，不显示。 */
  const coverShaped = computed(() => coverImageSize.value !== null);
  const coverBadge = computed<CoverBadge | null>(() =>
    coverShaped.value ? describeCoverBadge() : null,
  );
  /** 面板预览：优先本地（上传在途），上传成功后回落到线上地址。 */
  const coverDisplayPreviewUrl = computed(() => coverLocalPreviewUrl.value || coverPreviewUrl.value);

  const editArticleId = computed(() => {
    const fromArticleId = typeof route.query.article_id === "string" ? route.query.article_id.trim() : "";
    if (fromArticleId) return fromArticleId;
    const fromId = typeof route.query.id === "string" ? route.query.id.trim() : "";
    return fromId || "";
  });
  const isEditMode = computed(() => Boolean(editArticleId.value));
  const hydratedArticleId = ref("");

  const { data: tagOptions } = await useAsyncData("studio-write-tag-options", () => getTagOptions().catch(() => []));

  /**
   * 分类下拉的加载错误（#320）。
   *
   * 原来这里是 `.catch(() => [])`：接口一挂就和"这个账号还没有分类"长得一模一样，
   * 用户以为自己没分类，于是重复建同名，再撞上后端正确返回的"名称重复"。
   * 失败时把原因留在这里，由发布弹窗就地展示；空数组只代表真的为空。
   */
  const categoryOptionsError = ref("");

  /** loadCategoryOptions 是分类下拉的唯一取数入口：失败不抛，落进 categoryOptionsError。 */
  async function loadCategoryOptions(userId: string) {
    categoryOptionsError.value = "";
    try {
      return await getCategoryOptions(userId);
    } catch (error) {
      categoryOptionsError.value = errorMessage(error, "分类列表加载失败");
      return [];
    }
  }

  const { data: categoryOptions } = await useAsyncData(
    () => `studio-write-category-options:${currentUserId.value || "guest"}`,
    () => (currentUserId.value ? loadCategoryOptions(currentUserId.value) : Promise.resolve([])),
    {
      watch: [currentUserId],
    },
  );

  // 内联新建分类的进行中标记。放在本模块而不是 pendingState：
  // 后者是"发布/草稿/AI/封面"四条提交路径的锁，和分类下拉不是同一类状态。
  const categoryCreating = ref(false);

  /**
   * createCategoryInline 在写作页就地新建一个分类，成功后并入下拉并选中。
   *
   * 流程刻意拆成两段：
   *   1. 先调既有创建接口，让后端做归属、去空白与重名判定；
   *   2. 成功后再拉一次下拉（既有列表接口），按标题找到新分类并写回表单。
   * 不把步骤 2 的失败说成"创建失败"——分类其实已经建好了，只是列表没刷出来，
   * 两种失败的提示必须分开，否则用户会以为白建了一次。
   *
   * 返回值只表示"是否完成创建并选中"，失败原因已通过 message 播报。
   */
  async function createCategoryInline(rawTitle: string): Promise<boolean> {
    if (!isLoggedIn.value) {
      message.warning("请先登录后再新建分类");
      return false;
    }
    const titleError = categoryTitleError(rawTitle);
    if (titleError) {
      message.warning(titleError);
      return false;
    }

    const title = normalizeCategoryTitle(rawTitle);
    categoryCreating.value = true;
    let created = false;
    try {
      await createCategory(title);
      created = true;

      const options = await getCategoryOptions(currentUserId.value);
      categoryOptions.value = options;
      categoryOptionsError.value = "";
      const createdOption = findCategoryOptionByTitle(options, title);
      if (!createdOption) {
        message.warning("分类已创建，但未能载入新分类，请稍后重试。");
        return false;
      }

      form.category_id = createdOption.value;
      message.success(`已新建分类「${title}」`);
      return true;
    } catch (error) {
      message.error(
        created
          ? "分类已创建，但分类列表加载失败，请手动刷新后选择。"
          : `创建分类失败：${errorMessage(error, "请稍后重试")}`,
      );
      return false;
    } finally {
      categoryCreating.value = false;
    }
  }

  const { data: editingArticle, error: editingArticleError } = await useAsyncData(
    () => `studio-write-article-detail:${editArticleId.value || "new"}`,
    () => (editArticleId.value ? getArticleDetail(editArticleId.value) : Promise.resolve(null)),
    {
      watch: [editArticleId],
    },
  );

  watch(
    editingArticleError,
    (error) => {
      if (!error || !isEditMode.value) return;
      message.error(errorMessage(error, "加载待编辑文章失败"));
    },
    { immediate: true },
  );

  watch(
    [editArticleId, editingArticle],
    ([articleId, article]) => {
      if (!articleId) {
        hydratedArticleId.value = "";
        coverPreviewUrl.value = "";
        form.cover_image_id = null;
        form.visibility_status = "visible";
        coverDirty.value = false;
        coverUploadStage.value = "";
        coverError.value = "";
        resetCoverPickerState();
        return;
      }
      if (!article || hydratedArticleId.value === articleId) {
        return;
      }

      form.title = article.title || "";
      form.abstract = article.abstract || "";
      form.content = article.content || "";
      form.cover_image_id = article.cover_image_id ?? null;
      coverPreviewUrl.value = article.cover || "";
      coverDirty.value = false;
      // 回填的旧封面只有 URL、没有原图尺寸：不显示定形徽标。
      resetCoverPickerState();
      form.comments_toggle = article.comments_toggle ?? true;
      form.visibility_status = article.visibility_status === "user_hidden" ? "user_hidden" : "visible";
      // pending_review 不能写回表单（写接口只收两档），但要让页面知道
      // "已发布但还没对读者可见"，否则作者会以为设置里选的"可见"已经生效。
      pendingReview.value = article.visibility_status === "pending_review";
      form.category_id = article.category_id ?? null;
      form.tag_ids = Array.isArray(article.tag_ids) ? [...article.tag_ids] : [];
      hydratedArticleId.value = articleId;
    },
    { immediate: true },
  );

  const contentStats = computed(() => ({
    raw: form.content.length,
    pure: form.content.replace(/\s/g, "").length,
    lines: form.content ? form.content.split("\n").length : 1,
  }));

  const tocItems = computed(() => {
    return previewHeadings.value.map((item) => ({
      id: item.id,
      label: item.title,
      level: item.level,
    }));
  });

  const canSubmit = computed(() => Boolean(form.title.trim() && form.content.trim()));
  const draftActionLabel = computed(() => (isEditMode.value ? "更新草稿" : "保存草稿"));
  const publishActionLabel = computed(() => (isEditMode.value ? "更新并发布" : "确定并发布"));
  const isPublishingLocked = computed(() => pendingState.publish || pendingState.draft || pendingState.ai || pendingState.cover);
  const canOpenArticleScore = computed(() => Boolean(editArticleId.value));
  const currentArticlePublishStatus = computed(() => editingArticle.value?.publish_status ?? 0);

  const articleTitleModel = computed({
    get: () => form.title,
    set: (value: string) => {
      form.title = value;
    },
  });

  const articleContentModel = computed({
    get: () => form.content,
    set: (value: string) => {
      form.content = value;
    },
  });

  /** 释放定形产物的本地预览地址（不释放会一直占着这张图的内存）。 */
  function clearLocalCoverPreview() {
    const url = coverLocalPreviewUrl.value;
    coverLocalPreviewUrl.value = "";
    if (!url) return;
    try {
      if (typeof URL.revokeObjectURL === "function") URL.revokeObjectURL(url);
    } catch {
      // 已经释放过或环境不支持：忽略。
    }
  }

  function resetCoverPickerState() {
    clearLocalCoverPreview();
    coverPendingFile.value = null;
    coverImageSize.value = null;
    resetCoverCrop();
  }

  function openCoverFilePicker() {
    // 裁剪窗开着时不再开第二个文件框（一次只处理一张图）。
    if (pendingState.cover || coverCropRequest.value) {
      return;
    }
    coverFileInputRef.value?.click();
  }

  function clearCoverSelection() {
    form.cover_image_id = null;
    coverPreviewUrl.value = "";
    coverUploadStage.value = "";
    coverError.value = "";
    resetCoverPickerState();
    coverDirty.value = true;
    if (coverFileInputRef.value) {
      coverFileInputRef.value.value = "";
    }
  }

  /** 把定形产物传到图床；成功返回 true。原图不进入这条链路。 */
  async function uploadSelectedCover(file: File): Promise<boolean> {
    pendingState.cover = true;
    coverError.value = "";
    coverUploadStage.value = "开始上传封面…";
    try {
      const uploadResult = await uploadImageByTask(file, {
        onStage: mapCoverUploadStage,
        purpose: "cover",
      });
      if (!uploadResult.image_id || !uploadResult.url) {
        throw new Error("上传成功但未返回可用图片标识");
      }
      form.cover_image_id = uploadResult.image_id;
      coverPreviewUrl.value = uploadResult.url;
      clearLocalCoverPreview();
      coverDirty.value = true;
      coverUploadStage.value = "封面上传完成";
      message.success("封面上传成功");
      return true;
    } catch (error) {
      const text = errorMessage(error, "封面上传失败");
      coverUploadStage.value = "";
      coverError.value = text;
      message.error(text);
      return false;
    } finally {
      pendingState.cover = false;
    }
  }

  function mapCoverUploadStage(stage: "hashing" | "creating_task" | "uploading_to_qiniu" | "polling_status") {
    const textMap = {
      hashing: "正在计算文件指纹…",
      creating_task: "正在创建上传任务…",
      uploading_to_qiniu: "正在上传到对象存储…",
      polling_status: "正在确认图片状态…",
    } as const;
    coverUploadStage.value = textMap[stage];
  }

  function buildArticlePayload(status: 1 | 2) {
    return buildStudioArticlePayload(form, {
      status,
      isEditMode: isEditMode.value,
      coverDirty: coverDirty.value,
    });
  }

  /**
   * 选完封面文件后（票 215）：量原图尺寸 → 判比例。
   *
   * 与后台轮播、合集、项目封面上传走同一个顺序（`useImageCropUpload`）：
   *   - 与 16:9 相差 ≤1% → 自动居中定形成 1600×900，直接上传，不弹窗；
   *   - 相差 >1% → 只把原图交给裁剪窗，**不产出文件、不上传**，等作者确认或取消。
   *
   * 定形产物先落到预览上（作者看到的就是即将上传的那张图），再进入既有上传链路——
   * 原图永远不上传。取消裁剪等同取消本次上传：不留半成品封面。
   */
  async function handleCoverFileChange(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) {
      return;
    }

    if (!isImageFile(file)) {
      message.warning("请选择图片文件作为封面。");
      input.value = "";
      return;
    }

    coverError.value = "";
    coverUploadStage.value = "正在检查图片比例…";
    resetCoverPickerState();

    try {
      await coverCropFlow.select(file);
    } catch (error) {
      coverUploadStage.value = "";
      const text = errorMessage(error, "封面定形失败，请换一张图片");
      coverError.value = text;
      message.warning(text);
    } finally {
      input.value = "";
    }
  }

  /**
   * 裁剪窗确认：按作者选定的区域定形成**新文件**（仍是 1600×900 JPEG），再上传。
   * 原图不进入上传链路；裁剪参数不落任何记录。
   */
  async function confirmCoverCrop(framing: ShapeFramingState) {
    const request = coverCropRequest.value;
    if (!request) {
      return;
    }
    resetCoverCrop();
    coverError.value = "";
    coverUploadStage.value = `正在按选定区域定形成 ${COVER_SHAPE_RATIO_TEXT} 封面…`;
    try {
      await coverCropFlow.confirm(request, framing);
    } catch (error) {
      coverUploadStage.value = "";
      const text = errorMessage(error, "封面定形失败，请换一张图片");
      coverError.value = text;
      message.warning(text);
    } finally {
      if (coverFileInputRef.value) {
        coverFileInputRef.value.value = "";
      }
    }
  }

  /**
   * 裁剪窗取消：中止本次上传。现有封面（图片 ID 与线上地址）一律不动，
   * 也不产生任何记录——用户可以把这次选图当作没发生。
   */
  function cancelCoverCrop() {
    resetCoverCrop();
    coverUploadStage.value = "已取消本次上传，保留现有封面";
    coverError.value = "";
    if (coverFileInputRef.value) {
      coverFileInputRef.value.value = "";
    }
  }


  return {
    // 表单
    form,
    /** 当前文章是否仍在等内容检查通过才对读者可见（票 302）。 */
    pendingReview,
    articleTitleModel,
    articleContentModel,
    // 文章标识
    editArticleId,
    isEditMode,
    hydratedArticleId,
    // 数据
    tagOptions,
    categoryOptions,
    /** 分类下拉加载失败的可读原因；空串表示这次加载没出错（#320）。 */
    categoryOptionsError,
    categoryCreating,
    createCategoryInline,
    isLoggedIn,
    editingArticle,
    editingArticleError,
    // 封面
    coverFileInputRef,
    coverPreviewUrl,
    coverUploadStage,
    coverError,
    coverDirty,
    coverImageSize,
    coverShaped,
    coverBadge,
    coverDisplayPreviewUrl,
    openCoverFilePicker,
    clearCoverSelection,
    mapCoverUploadStage,
    handleCoverFileChange,
    // 裁剪窗（票 215）：比例不合规时才出现；取消＝中止本次上传
    coverCropRequest,
    coverCropPreviewUrl,
    confirmCoverCrop,
    cancelCoverCrop,
    buildArticlePayload,
    // 派生
    canSubmit,
    contentStats,
    tocItems,
    canOpenArticleScore,
    currentArticlePublishStatus,
    draftActionLabel,
    publishActionLabel,
    isPublishingLocked,
  };
}
