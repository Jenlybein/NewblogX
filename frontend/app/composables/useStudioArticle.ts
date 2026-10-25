import { computed, reactive, ref, watch, type Ref } from "vue";

import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { createArticle, getArticleDetail, updateArticle } from "~/services/article";
import { uploadImageByTask } from "~/services/image";
import { getCategoryOptions, getTagOptions } from "~/services/search";
import { useAuthStore } from "~/stores/auth";

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

/** Notifier 是本模块需要的提示能力（只声明用到的三个方法）。 */
export type Notifier = {
  success: (msg: string) => void;
  warning: (msg: string) => void;
  error: (msg: string) => void;
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
};

/**
 * useStudioArticle 提供文章内容与它的加载/回填/封面能力。
 *
 * "当前编辑的是哪篇文章"由 URL 决定（`?article_id=` 或 `?id=`），
 * 所以 editArticleId 与数据加载都在本模块内 —— 页面只关心
 * "有没有 articleId"这一个派生结果。
 */
export async function useStudioArticle(deps: StudioArticleDeps) {
  const { pendingState, message, previewHeadings } = deps;
  const route = useRoute();
  const authStore = useAuthStore();

  const currentUserId = computed(() => authStore.profileId || "");

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

  const coverDirty = ref(false);

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
    coverDirty.value = true;
    if (coverFileInputRef.value) {
      coverFileInputRef.value.value = "";
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
    const payload: {
      title: string;
      abstract?: string;
      content: string;
      category_id: string | null;
      tag_ids: string[];
      comments_toggle: boolean;
      visibility_status: "visible" | "user_hidden";
      status: 1 | 2;
      cover_image_id?: string | null;
    } = {
      title: form.title.trim(),
      abstract: form.abstract.trim() || undefined,
      content: form.content,
      category_id: form.category_id || null,
      tag_ids: form.tag_ids,
      comments_toggle: form.comments_toggle,
      visibility_status: form.visibility_status,
      status,
    };

    if (form.cover_image_id) {
      payload.cover_image_id = form.cover_image_id;
    } else if (isEditMode.value && coverDirty.value) {
      payload.cover_image_id = null;
    }

    return payload;
  }

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

    pendingState.cover = true;
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
      coverDirty.value = true;
      coverUploadStage.value = "封面上传完成";
      message.success("封面上传成功");
    } catch (error) {
      coverUploadStage.value = "";
      message.error(error instanceof Error ? error.message : "封面上传失败");
    } finally {
      pendingState.cover = false;
      input.value = "";
    }
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
    editingArticle,
    editingArticleError,
    // 封面
    coverFileInputRef,
    coverPreviewUrl,
    coverUploadStage,
    coverDirty,
    openCoverFilePicker,
    clearCoverSelection,
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
