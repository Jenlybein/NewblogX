import { useAsyncData, useRoute } from "#imports";
import { computed, reactive, ref, watch, type Ref } from "vue";

import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { getArticleDetail } from "~/services/article";
import { buildStudioArticlePayload } from "~/utils/studioArticlePayload";
import { uploadImageByTask } from "~/services/image";
import { getCategoryOptions, getTagOptions, createCategory } from "~/services/search";
import { useAuthStore } from "~/stores/auth";
import {
  clampFraming,
  framingStyle,
  type Framing,
  type Size,
} from "~/utils/coverFraming";
import {
  createCoverObjectUrl,
  readCoverImageSize,
  revokeCoverObjectUrl,
  type CoverImageSizeReader,
} from "~/utils/coverImageSize";
import {
  categoryTitleError,
  findCategoryOptionByTitle,
  normalizeCategoryTitle,
} from "~/utils/studioCategory";
import {
  describeCoverBadge,
  resolveCoverUploadRoute,
  type CoverBadge,
  type CoverUploadRoute,
} from "~/utils/studioCover";

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

export type StudioArticleDeps = {
  /** 进行中标记（封面流程要读写 cover）。 */
  pendingState: PendingState;
  /** 提示。 */
  message: Notifier;
  /** 预览区上报的标题锚点（目录与统计要用）。 */
  previewHeadings: Ref<ArticleHeadingAnchor[]>;
  /**
   * 读原图尺寸（可选注入）：
   * 默认实现走浏览器 Image 解码；单元测试注入替身即可驱动"合规/不合规"两条路径。
   */
  readImageSize?: CoverImageSizeReader;
};

/**
 * useStudioArticle 提供文章内容与它的加载/回填/封面能力。
 *
 * "当前编辑的是哪篇文章"由 URL 决定（`?article_id=` 或 `?id=`），
 * 所以 editArticleId 与数据加载都在本模块内 —— 页面只关心
 * "有没有 articleId"这一个派生结果。
 */
export async function useStudioArticle(deps: StudioArticleDeps) {
  const { pendingState, message, previewHeadings, readImageSize = readCoverImageSize } = deps;
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

  const coverFileInputRef = ref<HTMLInputElement | null>(null);

  const coverPreviewUrl = ref("");
  const coverUploadStage = ref("");
  /** 面板里留住的错误文案（上传失败不再只靠一闪而过的 toast）。 */
  const coverError = ref("");

  /** 本次选中的原图：取景确认后才上传；调整取景时还要用它重开取景窗。 */
  const coverPendingFile = ref<File | null>(null);
  /** 原图像素尺寸：决定走"直接上传"还是"先取景"。 */
  const coverImageSize = ref<Size | null>(null);
  /** 本地预览地址（取景窗前与上传在途都用它，所见即这张图）。 */
  const coverFramingPreviewUrl = ref("");
  /** 取景参数：本票只用于预览，不随文章载荷落库。 */
  const coverFraming = ref<Framing | null>(null);
  /** 取景窗是否打开（不合规时自动打开，也可手动调整）。 */
  const coverFramingOpen = ref(false);

  const coverDirty = ref(false);

  /** 这张图走哪条路；还没选图（或回填的旧封面没有尺寸）时为空串。 */
  const coverRoute = computed<CoverUploadRoute | "">(() =>
    coverImageSize.value ? resolveCoverUploadRoute(coverImageSize.value) : "",
  );
  const coverFramed = computed(() => coverFraming.value !== null);
  const coverBadge = computed<CoverBadge | null>(() =>
    coverRoute.value ? describeCoverBadge({ route: coverRoute.value, framed: coverFramed.value }) : null,
  );
  /** 面板预览：优先本地（取景/上传在途），上传成功后回落到线上地址。 */
  const coverDisplayPreviewUrl = computed(() => coverFramingPreviewUrl.value || coverPreviewUrl.value);
  /** 预览图的取景样式（与取景窗、线上渲染同一个出口）。 */
  const coverPreviewStyle = computed(() => (coverFraming.value ? framingStyle(coverFraming.value) : null));

  const editArticleId = computed(() => {
    const fromArticleId = typeof route.query.article_id === "string" ? route.query.article_id.trim() : "";
    if (fromArticleId) return fromArticleId;
    const fromId = typeof route.query.id === "string" ? route.query.id.trim() : "";
    return fromId || "";
  });
  const isEditMode = computed(() => Boolean(editArticleId.value));
  const hydratedArticleId = ref("");

  const { data: tagOptions } = await useAsyncData("studio-write-tag-options", () => getTagOptions().catch(() => []));
  const { data: categoryOptions } = await useAsyncData(
    () => `studio-write-category-options:${currentUserId.value || "guest"}`,
    () => (currentUserId.value ? getCategoryOptions(currentUserId.value).catch(() => []) : Promise.resolve([])),
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
          : `创建分类失败：${error instanceof Error ? error.message : "请稍后重试"}`,
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
      message.error(error instanceof Error ? error.message : "加载待编辑文章失败");
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
      // 回填的旧封面只有 URL、没有原图尺寸：不显示合规徽标，也不假装取过景。
      resetCoverPickerState();
      form.comments_toggle = article.comments_toggle ?? true;
      form.visibility_status = article.visibility_status === "user_hidden" ? "user_hidden" : "visible";
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

  function clearLocalCoverPreview() {
    revokeCoverObjectUrl(coverFramingPreviewUrl.value);
    coverFramingPreviewUrl.value = "";
  }

  function resetCoverPickerState() {
    clearLocalCoverPreview();
    coverPendingFile.value = null;
    coverImageSize.value = null;
    coverFraming.value = null;
    coverFramingOpen.value = false;
  }

  function openCoverFilePicker() {
    if (pendingState.cover) {
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

 /** 手动取景 / 调整取景：只有选过图（读得到尺寸）才能打开。 */
  function openCoverFraming() {
    if (!coverPendingFile.value || !coverImageSize.value) {
      message.warning("请先选择一张图片，才能调整取景。");
      return;
    }
    if (!coverFramingPreviewUrl.value) {
      coverFramingPreviewUrl.value = createCoverObjectUrl(coverPendingFile.value);
    }
    coverFramingOpen.value = true;
  }

  /** 取消取景：强制路径（比例不合规且还没取过景）＝中止本次上传。 */
  function cancelCoverFraming() {
    const forced = coverRoute.value === "frame" && !coverFramed.value;
    coverFramingOpen.value = false;
    clearLocalCoverPreview();
    coverUploadStage.value = "";
    if (!forced) {
      // 调整已有取景时取消只是关窗：已经确认过的构图与封面都保持不变。
      message.info("已关闭取景窗，封面保持不变");
      return;
    }
    coverPendingFile.value = null;
    coverImageSize.value = null;
    coverFraming.value = null;
    coverError.value = "";
    message.info("已取消取景，本次上传未开始：没有产生图片，也没有产生文章封面");
  }

  /** 确认取景：参数只用于预览；随后立即上传这张原图。 */
  async function confirmCoverFraming(framing: Framing) {
    coverFramingOpen.value = false;
    const file = coverPendingFile.value;
    if (!file) {
      message.warning("取景已确认，但找不到待上传的图片，请重新选择。");
      return;
    }
    const safe = clampFraming(framing);
    coverUploadStage.value = "取景已确认，正在上传封面…";
    const uploaded = await uploadSelectedCover(file);
    if (uploaded) {
      coverFraming.value = safe;
      coverUploadStage.value = "封面上传完成";
    }
  }

  /** 把选中的原图传到图床；成功返回 true，并由调用方决定是否记取景。 */
  async function uploadSelectedCover(file: File): Promise<boolean> {
    pendingState.cover = true;
    coverError.value = "";
    coverUploadStage.value = "开始上传封面…";
    try {
      const uploadResult = await uploadImageByTask(file, {
        onStage: mapCoverUploadStage,
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
      const text = error instanceof Error ? error.message : "封面上传失败";
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
    return buildStudioArticlePayload(form, { status, isEditMode: isEditMode.value, coverDirty: coverDirty.value });
  }

  /**
   * 选完封面文件后的分叉（与后台轮播上传同一套口径）：
   *   - 16:9 合规 → 直接上传；
   *   - 不合规 → 只打开取景窗，确认后才上传；取消即中止，不产生图片。
   * 尺寸读不出来时直接让用户换图：没有尺寸就无法判断会不会露白。
   */
  async function handleCoverFileChange(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) {
      return;
    }

    if (!file.type.startsWith("image/")) {
      message.warning("请选择图片文件作为封面。");
      input.value = "";
      return;
    }

    coverError.value = "";
    coverUploadStage.value = "";
    resetCoverPickerState();
    coverPendingFile.value = file;

    const size = await readImageSize(file).catch(() => null);
    if (!size) {
      coverPendingFile.value = null;
      input.value = "";
      message.warning("读不到这张图片的尺寸，请换一张图片作为封面。");
      return;
    }

    coverImageSize.value = size;
    coverFramingPreviewUrl.value = createCoverObjectUrl(file);

    if (resolveCoverUploadRoute(size) === "direct") {
      await uploadSelectedCover(file);
      input.value = "";
      return;
    }

    coverUploadStage.value = "比例不合规，先确认取景再上传";
    coverFramingOpen.value = true;
    input.value = "";
  }


  return {
    // 表单
    form,
    articleTitleModel,
    articleContentModel,
    // 文章标识
    editArticleId,
    isEditMode,
    hydratedArticleId,
    // 数据
    tagOptions,
    categoryOptions,
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
    coverRoute,
    coverBadge,
    coverFramed,
    coverFramingOpen,
    coverFramingPreviewUrl,
    coverDisplayPreviewUrl,
    coverPreviewStyle,
    openCoverFilePicker,
    clearCoverSelection,
    openCoverFraming,
    confirmCoverFraming,
    cancelCoverFraming,
    mapCoverUploadStage,
    handleCoverFileChange,
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
