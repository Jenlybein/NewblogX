import { ref } from "vue";

import { createArticle, updateArticle } from "~/services/article";
import { ApiBusinessError } from "~/services/http/errors";
import {
  DRAFT_DISABLED_NOTICE,
  PUBLISH_DISABLED_NOTICE,
  type ArticleWritePolicy,
} from "~/utils/writePolicy";
import { errorMessage } from "~/utils/errorMessage";

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

/**
 * QualityRejectionIssue 是评分给出的一条问题（定位 + 原因 + 建议）。
 *
 * 与后端 `QualityIssue` 一一对应；这里手写是因为它只出现在**失败响应的
 * data** 里，契约的 Envelope 泛型不覆盖错误分支（见 contracts.md §6.5）。
 */
export type QualityRejectionIssue = {
  reason?: string;
  suggestion?: string;
  positions?: Array<{ paragraph?: number; quote?: string }>;
};

/** QualityRejectionDetails 是「质量不达标」的完整拒绝数据。 */
export type QualityRejectionDetails = {
  article_id?: string;
  score: number;
  required_score: number;
  gap: number;
  level?: string;
  issues?: QualityRejectionIssue[];
};

/**
 * qualityRejectionOf 从错误里取出质量拒绝数据；不是这类错误时返回 null。
 *
 * 用鸭子类型而不是 `instanceof ApiBusinessError`：跨模块边界传进来的错误
 * 可能是被包装过的，只认 `data` 的形状更稳。
 */
export function qualityRejectionOf(reason: unknown): QualityRejectionDetails | null {
  const data =
    reason instanceof ApiBusinessError
      ? reason.data
      : (reason as { data?: unknown } | null)?.data;
  if (!data || typeof data !== "object") return null;
  const candidate = data as Partial<QualityRejectionDetails>;
  if (typeof candidate.score !== "number" || typeof candidate.required_score !== "number") {
    return null;
  }
  return candidate as QualityRejectionDetails;
}

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
  /**
   * 站点「发布与展示」的两个写作开关（可选）。
   *
   * 缺省视为全开：服务端才是判定边界，这里只是"点了立刻告诉用户为什么不行"，
   * 不能因为调用方没传就改变原有行为。
   */
  policy?: { value: ArticleWritePolicy };
  /**
   * 质量不达标时的展示回调（可选）。
   *
   * 不传就退回一句纯文本提示；传了则由页面把分数、差额与问题列表完整呈现
   * （#306 验收标准：作者要能看到"按哪几条改"）。
   */
  onQualityRejected?: (details: QualityRejectionDetails) => void;
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
    policy,
    onQualityRejected,
  } = deps;

  /**
   * 评分中标记：开启质量闸门时，一次提交要等 5–30 秒模型调用。
   *
   * 它和 pendingState.publish 不是同一件事：后者在整条提交流程里都为 true，
   * 前者只在"等评分"这一段为 true，页面据此把按钮文案换成「评分中…」，
   * 让作者知道现在慢是因为在评分，而不是卡住了。
   */
  const scoring = ref(false);

  /** 站点策略是否允许这次提交（没传 policy 时按允许处理）。 */
  function sitePolicyAllows(status: 1 | 2): boolean {
    if (!policy) return true;
    return status === 1 ? policy.value.allowDraft : policy.value.allowPublish;
  }

  /** 被站点策略拦下时的说明：说清是站点策略，不是账号问题。 */
  function sitePolicyNotice(status: 1 | 2): string {
    return status === 1 ? DRAFT_DISABLED_NOTICE : PUBLISH_DISABLED_NOTICE;
  }

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

    // 站点策略：发布/草稿开关关闭时直接说明原因，不发请求。
    // 这只是体验层，服务端仍会用自己的判定拒绝（唯一边界）。
    if (!sitePolicyAllows(status)) {
      message.warning(sitePolicyNotice(status));
      return;
    }

    const key = status === 1 ? "draft" : "publish";
    pendingState[key] = true;
    // 发布（状态 2）会在开启闸门时先评分；草稿不评分。
    scoring.value = status === 2;

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
        // 已发布但可能仍处 pending_review：作者立刻拿到"已发布"回执，
        // 对读者可见要等模型判定通过。文案不能承诺"立刻公开"。
        message.success("文章已发布，内容检查通过后将对读者可见");
      } else if (created.publish_status === 2) {
        message.success("文章已提交审核，审核通过后会公开展示");
      } else {
        message.success("文章已提交发布");
      }
      await router.push(`/article/${created.id}`);
    } catch (error) {
      // 质量不达标是**带数据的业务拒绝**，不是"提交失败"：把分数、差额与
      // 问题列表交给页面呈现，作者才有下一步动作可做（#306 验收标准）。
      const rejection = qualityRejectionOf(error);
      if (rejection) {
        if (onQualityRejected) {
          onQualityRejected(rejection);
        } else {
          message.error(
            `内容质量未达标：${rejection.score} 分，放行线 ${rejection.required_score} 分，还差 ${rejection.gap} 分。`,
          );
        }
        return;
      }
      message.error(errorMessage(error, "文章提交失败"));
    } finally {
      pendingState[key] = false;
      scoring.value = false;
    }
  }

  return { submitArticle, scoring };
}
