import type { Ref } from "vue";

import { getArticleScoreDetail, regenerateArticleScore } from "~/services/ai";

// 文章质量评分弹窗的数据加载。
//
// 从 pages/studio/write.vue 抽出来（M6 ⑧）。三个入口（打开弹窗取历史、
// 手动刷新、评分未生成时补一次）共用同一份数据与同一套 loading 状态。
// 混在页面里时，"刷新中忘了复位 loading"或"打开时没清掉上一篇的评分"
// 这类问题只能靠人工复现。
//
// 状态仍由页面持有（ref 注入），本模块只负责怎么取、什么时候清空 ——
// 页面是状态的唯一持有者。

/** ArticleScore 是评分接口的返回结构。 */
export type ArticleScore = Awaited<ReturnType<typeof getArticleScoreDetail>>;

/** Notifier 是本模块需要的提示能力。 */
export type Notifier = {
  success: (msg: string) => void;
  warning: (msg: string) => void;
  info: (msg: string) => void;
  error: (msg: string) => void;
};

export type StudioScoreDeps = {
  modalOpen: Ref<boolean>;
  loading: Ref<boolean>;
  refreshing: Ref<boolean>;
  data: Ref<ArticleScore | null>;
  /** 当前编辑的文章 id（空串表示新建）。 */
  articleId: Ref<string>;
  message: Notifier;
};

export function useStudioScore(deps: StudioScoreDeps) {
  const {
    modalOpen: articleScoreModalOpen,
    loading: articleScoreLoading,
    refreshing: articleScoreRefreshing,
    data: articleScoreData,
    articleId: editArticleId,
    message,
  } = deps;

  async function fetchStoredArticleScore() {
    if (!editArticleId.value) {
      throw new Error("请先保存当前文章，再查看历史评分。");
    }

    articleScoreData.value = await getArticleScoreDetail(editArticleId.value);
  }

  async function handleOpenArticleScore() {
    if (!editArticleId.value) {
      message.info("新建文章暂时没有 article_id，请先保存为草稿或更新文章后再查看评分。");
      return;
    }

    articleScoreModalOpen.value = true;
    articleScoreLoading.value = true;
    try {
      await fetchStoredArticleScore();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "读取评分失败");
    } finally {
      articleScoreLoading.value = false;
    }
  }


  return { fetchStoredArticleScore, handleOpenArticleScore };
}
