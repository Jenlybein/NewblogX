import { createArticle, updateArticle } from "~/services/article";

// 文章提交：存草稿 / 发布 / 更新。
//
// 从 pages/studio/write.vue 抽出来（M6 ⑧）。这里的判断分支多且都"看起来一样"：
// 新建 vs 编辑、草稿 vs 发布、发布后是直接可见还是要审核 —— 三种二的组合共 6 条路径，
// 每条的错误提示都不一样（用户会照着提示去操作，说错了就是误导）。
// 抽成独立模块后，这些分支集中在一处，改文案不会漏掉某一支。
//
// 依赖全部注入：本模块不持有状态，只负责"按当前状态决定做什么"。

/** Notifier 是本模块需要的提示能力。 */
export type Notifier = {
  success: (msg: string) => void;
  warning: (msg: string) => void;
  error: (msg: string) => void;
};

/** ArticlePayload 是提交给创建/更新接口的载荷。 */
export type ArticlePayload = Parameters<typeof createArticle>[0];

export type StudioPublishDeps = {
  /** 标题与正文是否都非空。 */
  canSubmit: { value: boolean };
  /** 进行中标记。 */
  pendingState: { draft: boolean; publish: boolean; ai: boolean; cover: boolean };
  /** 当前是否编辑已有文章，以及它的 id。 */
  isEditMode: { value: boolean };
  editArticleId: { value: string };
  /** 发布弹窗开关。 */
  publishVisible: { value: boolean };
  /** 当前可见性（决定发布后的提示文案）。 */
  visibilityStatus: () => "visible" | "user_hidden";
  /** 组装载荷。 */
  buildPayload: (status: 1 | 2) => ArticlePayload;
  message: Notifier;
  router: { push: (to: string) => Promise<unknown> };
};

export function useStudioPublish(deps: StudioPublishDeps) {
  const {
    canSubmit,
    pendingState,
    isEditMode,
    editArticleId,
    publishVisible,
    visibilityStatus: formVisibilityStatus,
    buildPayload: buildArticlePayload,
    message,
    router,
  } = deps;

  // 页面里原本直接读 form.visibility_status，这里换成回调，避免把整个表单传进来。
  const form = {
    get visibility_status() {
      return formVisibilityStatus();
    },
  };

  async function submitArticle(status: 1 | 2) {
    if (!canSubmit.value) {
      message.warning("标题和正文是必填项。");
      return;
    }

    if (pendingState.cover) {
      message.warning("封面仍在上传中，请稍后再提交。");
      return;
    }

    const key = status === 1 ? "draft" : "publish";
    pendingState[key] = true;

    try {
      const payload = buildArticlePayload(status);

      if (isEditMode.value && editArticleId.value) {
        await updateArticle(editArticleId.value, payload);
        publishVisible.value = false;
        if (status === 1) {
          message.success("草稿已更新");
        } else {
          message.success(
            form.visibility_status === "user_hidden" ? "文章已提交发布（当前为仅自己可见）" : "文章已提交发布（可能进入审核）",
          );
        }
        await router.push(`/article/${editArticleId.value}`);
        return;
      }

      const created = await createArticle(payload);
      publishVisible.value = false;
      if (status === 1) {
        message.success("草稿已保存");
      } else if (created.publish_status === 3) {
        message.success("文章已发布成功");
      } else if (created.publish_status === 2) {
        message.success("文章已提交审核，审核通过后会公开展示");
      } else {
        message.success("文章已提交发布");
      }
      await router.push(`/article/${created.id}`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "文章提交失败");
    } finally {
      pendingState[key] = false;
    }
  }

  return { submitArticle };
}
