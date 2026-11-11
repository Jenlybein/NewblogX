import { formatCount } from "~/utils/format";

/**
 * 阅读页互动条的文案（票 236 / 决定清单第 2 节）。
 *
 * 互动条是**纯图标 + 数字**（右栏 320px、窄屏底部条还要让出安全区），可见文字省掉了，
 * 所以读屏名称与悬停提示是这个组件唯一的语义出口。抽成纯函数有三个理由：
 *
 *  1. 文案依赖"当前是什么态"（已赞 / 已藏 / 在途），而态来自 `useArticleDigg` 与正文；
 *     放在模板里就是四份嵌套三元，改一处漏三处；
 *  2. 计数走站内既有的 `formatCount` 口径（1.2k / 1.2w），不能每条自己写一遍缩写；
 *  3. 文案是可脱离组件断言的规格（`test/utils/articleActions.test.ts`）。
 */

export type ArticleActionKind = "digg" | "favor" | "share" | "comment" | "bottom" | "top";

export interface ArticleActionCopyOptions {
  kind: ArticleActionKind;
  count: number;
  /** 已赞 / 已藏：点下去是**撤销**，文案必须说出来。 */
  active?: boolean;
  /** 写请求在途：此时按下去会切到哪一侧还不确定，不能念旧态。 */
  pending?: boolean;
}

export interface ArticleActionCopy {
  /** 无障碍名称（`aria-label`）。 */
  label: string;
  /** 悬停提示（`title`）：短，只说动作本身。 */
  title: string;
}

/** 计数兜底：负数 / NaN / 缺省一律按 0，不把 NaN 念给读屏。 */
function safeCount(count: number): number {
  const value = Number(count);
  if (!Number.isFinite(value) || value < 0) {
    return 0;
  }
  return Math.trunc(value);
}

/** 生成某个互动动作的读屏名称与悬停提示。 */
export function describeArticleAction(options: ArticleActionCopyOptions): ArticleActionCopy {
  const count = formatCount(safeCount(options.count));

  switch (options.kind) {
    case "digg": {
      if (options.pending) {
        return { label: `点赞处理中，当前 ${count} 个赞`, title: "点赞处理中" };
      }
      return options.active
        ? { label: `取消点赞，当前 ${count} 个赞`, title: "取消点赞" }
        : { label: `点赞，当前 ${count} 个赞`, title: "点赞" };
    }
    case "favor":
      return options.active
        ? { label: `已收藏，点按可移除，当前 ${count} 次收藏`, title: "已收藏" }
        : { label: `收藏，当前 ${count} 次收藏`, title: "收藏" };
    case "share":
      return { label: "分享这篇文章", title: "分享" };
    case "comment":
      return { label: `打开评论输入框，当前 ${count} 条评论`, title: "评论" };
    case "bottom":
      return { label: "跳到页面底部（评论区）", title: "到底" };
    case "top":
      return { label: "回到页面顶部", title: "到顶" };
  }
}
