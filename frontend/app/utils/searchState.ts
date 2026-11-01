// 搜索页空态 / 错误态的文案与下一步指引。
//
// 为什么是纯函数：搜索页有五种"没内容"的情形（加载中、请求失败、还没输入、
// 智能搜索没输入、确实没匹配），如果直接在模板里堆 v-if 文案，会漏掉"下一步
// 该做什么"。这里把状态收敛成一个可单测的判定函数，页面只负责把返回的
// actions 渲染成按钮。
export type SearchMode = "normal" | "ai";

export type SearchAction = "retry" | "clear-filters" | "switch-mode" | "open-home";

export const SEARCH_ACTION_LABELS: Record<SearchAction, string> = {
  retry: "重试本页",
  "clear-filters": "清空条件",
  "switch-mode": "换个搜索方式",
  "open-home": "回到首页",
};

export type SearchEmptyKind = "loading" | "error" | "idle" | "ai-needs-query" | "no-match";

export interface SearchEmptyState {
  kind: SearchEmptyKind;
  title: string;
  description: string;
  /** 下一步可做的事（列表，给读者看的人话指引）。 */
  hints: string[];
  actions: SearchAction[];
}

export interface SearchEmptyContext {
  mode: SearchMode;
  key: string;
  tagId: string;
  pending: boolean;
  failed: boolean;
}

export function resolveSearchEmptyState(context: SearchEmptyContext): SearchEmptyState {
  const trimmedKey = context.key.trim();
  const isAi = context.mode === "ai";

  if (context.pending) {
    return {
      kind: "loading",
      title: isAi ? "正在理解你的问题" : "正在搜索文章",
      description: isAi ? "系统正在把问题转成检索条件，请稍候。" : "正在读取匹配结果，请稍候。",
      hints: [],
      actions: [],
    };
  }

  if (context.failed) {
    return {
      kind: "error",
      title: "搜索暂时不可用",
      description: isAi
        ? "智能搜索没有返回结果，可能是网络或后端暂时不可用。"
        : "文章搜索没有返回结果，可能是网络或后端暂时不可用。",
      hints: ["先重试本页；仍然失败就清空条件换个关键词。", "也可以切换到另一种搜索方式再试一次。"],
      actions: ["retry", "clear-filters", "switch-mode", "open-home"],
    };
  }

  if (isAi && !trimmedKey) {
    return {
      kind: "ai-needs-query",
      title: "先描述你想找什么",
      description: "智能搜索会把一句自然语言变成检索条件，直接在搜索框里描述即可。",
      hints: ["例如：想找讲 Nuxt 部署的长文、或者最近的 AI 实践总结。"],
      actions: ["switch-mode", "open-home"],
    };
  }

  if (!isAi && !trimmedKey && !context.tagId) {
    return {
      kind: "idle",
      title: "输入关键词开始搜索",
      // 顶部说明段已删（#126）；这里不再复述"能搜标题/摘要/正文"与"切到智能搜索"，
      // 引导交给下面的 hints 与 actions，空态只说下一步该做什么。
      description: "",
      hints: ["关键词越具体，结果越准。", "标签筛选适合想按主题浏览时的场景。"],
      actions: ["switch-mode", "open-home"],
    };
  }

  return {
    kind: "no-match",
    title: "没有匹配的文章",
    description: isAi
      ? "这句描述没有匹配到站内文章，换一种更具体的说法会更容易命中。"
      : "当前关键词与筛选条件没有匹配到公开文章。",
    hints: ["清空标签或换成更宽泛的关键词再试。", "搜索是在公开文章范围内进行的，未发布内容不会出现。"],
    actions: ["clear-filters", "switch-mode", "open-home"],
  };
}
