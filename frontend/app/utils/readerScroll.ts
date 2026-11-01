/**
 * 读者站的路由滚动行为（纯函数版本，便于单测；接线在 `app/router.options.ts`）。
 *
 * spec #79 用户故事 3：从文章页点「返回首页」要**停在原来的滚动位置**。
 * Vue Router 在浏览器前进/后退（含站内 SPA 返回）时会给出 `savedPosition`；
 * 其它导航（点链接去新页面、首屏进入）一律回顶。
 *
 * 刻意不处理 hash 锚点：读者站没有 `#hash` 路由消费点（文章目录跳转由
 * `scrollToHeading` 直接操作滚动容器），默认 Nuxt 的 hash 行为在这里不适用。
 */
export interface ReaderScrollOffset {
  left: number;
  top: number;
}

export function resolveReaderScrollPosition(
  savedPosition: Partial<ReaderScrollOffset> | null | undefined,
): ReaderScrollOffset {
  if (savedPosition && Number.isFinite(savedPosition.top) && Number.isFinite(savedPosition.left)) {
    return { left: savedPosition.left as number, top: savedPosition.top as number };
  }

  return { left: 0, top: 0 };
}
