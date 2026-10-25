import { shallowRef, watch } from "vue";

/**
 * 文章点赞的状态机。
 *
 * # 为什么它值得单独一个 composable
 *
 * 这块逻辑看着只是"点一下后端取反"，实际有四个容易错的地方，混在 791 行的页面里
 * 没人愿意通读：
 *
 *  1. **乐观更新 + 脏标记**：请求返回前先把界面改掉；但 `article` 还会被别的地方
 *     refresh，一旦刷新回来就会把乐观值覆盖回旧值 —— 所以要有 `touched` 标记，
 *     触碰过之后就不再接受服务端值，直到文章换了（articleId 变了）才复位。
 *  2. **结果靠文案判断**：接口返回的是 `{msg}`，后端用"点赞成功 / 取消点赞成功"
 *     区分方向。这里按文案猜方向，猜不到就按"取反"兜底 —— 所以必须有兜底，
 *     否则后端改文案就静默错。
 *  3. **计数不能为负**：并发或重复点击时 delta 可能把 0 减成 -1。
 *  4. **防抖**：连点不该连发请求，也不该让按钮闪。
 *
 * # 依赖为什么用 getter / 回调而不是 ref
 *
 * 页面的 `article` 是 `useAsyncData` 出来的 `Ref<ArticleDetail | null>`。直接把它
 * 传进来会因为 Ref 可写而要求类型完全一致（不变性），等于把这个 composable
 * 和 api 类型绑死。用 getter + 回调，这里只关心两个字段，页面也不必暴露整个 ref。
 */

/** 本模块只关心文章上的两个点赞字段。 */
export type ArticleDiggSnapshot = {
  isDigg?: boolean;
  diggCount?: number;
};

export type ArticleDiggDeps = {
  /** 当前文章 id（换成别的文章时要复位脏标记）。 */
  articleId: () => string;
  /** 从文章里取点赞字段。 */
  snapshot: () => ArticleDiggSnapshot;
  /** 把点赞结果写回文章（页面负责实际赋值）。 */
  patchArticle: (isDigg: boolean, diggCount: number) => void;
  /** 触发后端切换。 */
  toggle: (articleId: string) => Promise<{ msg?: string }>;
  /** 登录校验：未登录时打开登录弹窗并中止。 */
  ensureLoggedIn: () => Promise<boolean>;
  openAuthModal: () => void;
  notify: {
    success: (msg: string) => void;
    error: (msg: string) => void;
  };
  /** 连点防抖窗口，默认 600ms。 */
  debounceMs?: number;
};

/**
 * 解析后端返回文案，得到"这次是点赞还是取消 + 计数增量"。
 *
 * 抽成导出函数是为了能单测：它靠中文字符串猜方向，本质上是脆弱的，
 * 必须有测试钉住当前行为，将来后端换成返回布尔值时才敢改。
 */
export function resolveDiggMutation(resultMessage: string, wasDigged: boolean): { isDigged: boolean; countDelta: number } {
  if (resultMessage.includes("取消")) {
    return { isDigged: false, countDelta: -1 };
  }
  if (resultMessage.includes("点赞")) {
    return { isDigged: true, countDelta: 1 };
  }
  // 文案改了就按"取反"兜底：至少方向不会反，代价是文案变了要改这里。
  return { isDigged: !wasDigged, countDelta: wasDigged ? -1 : 1 };
}

/**
 * 创建点赞状态机。
 *
 * @param deps 见 {@link ArticleDiggDeps}
 */
export function useArticleDigg(deps: ArticleDiggDeps) {
  const debounceMs = deps.debounceMs ?? 600;

  const isDigged = shallowRef(false);
  const diggCount = shallowRef(0);
  /** 用户手动点过之后，不再接受服务端刷新回来的值（避免乐观更新被覆盖）。 */
  const touched = shallowRef(false);
  const boundArticleId = shallowRef("");
  const pending = shallowRef(false);
  const debouncing = shallowRef(false);
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  // 文章换了就复位；同一篇文章内，只有没被"用户操作过"时才跟随服务端值。
  watch(
    () => {
      const snap = deps.snapshot();
      return [deps.articleId(), snap.isDigg, snap.diggCount] as const;
    },
    ([currentId, serverIsDigged, serverCount]) => {
      if (currentId !== boundArticleId.value) {
        boundArticleId.value = currentId;
        touched.value = false;
      }
      if (touched.value) {
        return;
      }
      isDigged.value = Boolean(serverIsDigged);
      diggCount.value = Math.max(Number(serverCount ?? 0), 0);
    },
    { immediate: true },
  );

  function patch(nextIsDigged: boolean, nextCount: number) {
    isDigged.value = nextIsDigged;
    diggCount.value = Math.max(nextCount, 0);
    deps.patchArticle(nextIsDigged, diggCount.value);
  }

  function startDebounce() {    debouncing.value = true;
    if (debounceTimer) {
      clearTimeout(debounceTimer);
    }
    debounceTimer = setTimeout(() => {
      debouncing.value = false;
      debounceTimer = null;
    }, debounceMs);
  }

  async function handleLike() {
    const loggedIn = await deps.ensureLoggedIn();
    if (!loggedIn) {
      deps.openAuthModal();
      return;
    }
    if (pending.value || debouncing.value) {
      return;
    }

    const wasDigged = isDigged.value;
    pending.value = true;
    try {
      const result = await deps.toggle(deps.articleId());
      const resultMessage = result.msg || (wasDigged ? "取消点赞成功" : "点赞成功");
      const mutation = resolveDiggMutation(resultMessage, wasDigged);
      touched.value = true;
      patch(mutation.isDigged, diggCount.value + mutation.countDelta);
      deps.notify.success(resultMessage);
    } catch (error) {
      deps.notify.error(error instanceof Error ? error.message : "点赞失败");
    } finally {
      pending.value = false;
      startDebounce();
    }
  }

  /** 清掉防抖定时器：组件卸载时必须调用，否则定时器会在已销毁的组件上改状态。 */
  function dispose() {
    if (debounceTimer) {
      clearTimeout(debounceTimer);
      debounceTimer = null;
    }
    debouncing.value = false;
  }

  return {
    articleIsDigged: isDigged,
    articleDiggCount: diggCount,
    articleLikePending: pending,
    /** 按钮在防抖窗口内应禁用（否则会连发请求）。 */
    articleLikeDebouncing: debouncing,
    /** 用户是否已经手动改过点赞状态（页面用它决定要不要再拉服务端值）。 */
    articleDiggTouched: touched,
    handleLike,
    /** 卸载时清掉定时器（页面 onUnmounted 调用）。 */
    dispose,
  };
}
