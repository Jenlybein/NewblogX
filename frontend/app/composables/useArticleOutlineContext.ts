import { computed, onMounted, ref, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";

import { getArticleOutlineContext } from "~/services/outline";
import type { ArticleOutlineContext } from "~/utils/outlineContext";
import { detectReaderRenderMode, type ReaderRenderMode } from "~/utils/readerRenderMode";

/**
 * 阅读页左栏目录树的取数时序（票 220 / 决定清单 A9–A11）。
 *
 * 三条约束：
 *  1. **树只在客户端取数**：`/article/**` 的 SSR 边界与匿名缓存策略不变（ADR-0016），
 *     SSR 阶段一次也不发这条请求；
 *  2. 站内跳转先出骨架：client navigation 下同步进入 `pending`，页面骨架与
 *     真实三栏版式同构，不引发布局跳动；
 *  3. 无归属 / 取数失败一律降级为「不显示左栏」：右栏标题大纲与阅读进度不受影响。
 *
 * 文章不归属任何树时后端返回 `has_tree=false`，这里变成 `empty`，页面据此不渲染左栏。
 */

export type ArticleOutlineStatus = "idle" | "pending" | "ready" | "empty";

export interface ArticleOutlineContextOptions {
  articleId: MaybeRefOrGetter<string>;
  /** 阅读页 URL 上的 `?from=`：指向"我从哪棵树来"，原样透传给后端。 */
  from?: MaybeRefOrGetter<string | undefined>;
  /** 覆盖运行环境判定（单测注入）；默认按 import.meta + isHydrating 判定。 */
  mode?: ReaderRenderMode;
  /** 取数入口可注入（单测用假请求）。 */
  fetch?: (articleId: string, from?: string) => Promise<ArticleOutlineContext>;
}

export function useArticleOutlineContext(options: ArticleOutlineContextOptions) {
  const mode = options.mode ?? detectReaderRenderMode();
  const request = options.fetch ?? getArticleOutlineContext;

  const context = ref<ArticleOutlineContext | null>(null);
  // hydration 首屏必须与 SSR 输出一致（SSR 没有左栏），所以从 idle 起步；
  // 站内跳转没有既有 DOM，直接 pending，骨架与真实三栏版式同构。
  const status = ref<ArticleOutlineStatus>(
    mode === "server" || mode === "hydration" ? "idle" : "pending",
  );

  // 来源/文章切换时旧响应可能后到：只认最后一次请求。
  let sequence = 0;

  const currentArticleId = () => String(toValue(options.articleId) ?? "").trim();
  const currentFrom = () => String(toValue(options.from) ?? "");

  async function load(): Promise<void> {
    const articleId = currentArticleId();
    // SSR 不取数：服务端渲染边界与按 URL 键的匿名缓存不得因左栏变宽（ADR-0016）。
    if (mode === "server" || !articleId) {
      status.value = "idle";
      return;
    }

    const requestId = ++sequence;
    status.value = "pending";
    try {
      const result = await request(articleId, currentFrom());
      if (requestId !== sequence) {
        return;
      }
      if (result?.has_tree) {
        context.value = result;
        status.value = "ready";
        return;
      }
      context.value = null;
      status.value = "empty";
    } catch {
      // 左栏是次要内容：失败只是不显示，不把整页带崩。
      if (requestId !== sequence) {
        return;
      }
      context.value = null;
      status.value = "empty";
    }
  }

  onMounted(() => {
    void load();
  });

  watch([currentArticleId, currentFrom], () => {
    if (mode === "server") {
      return;
    }
    void load();
  });

  const pending = computed(() => status.value === "pending");
  /** 有可展示的树（左栏出现）。 */
  const hasTree = computed(() => context.value?.has_tree === true);
  /** 左栏占位是否应出现：pending 时先占位，ready 时是真实树；empty 时整栏不出现。 */
  const active = computed(() => pending.value || hasTree.value);

  return { context, status, pending, hasTree, active, load };
}
