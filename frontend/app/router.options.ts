import type { RouterConfig } from "@nuxt/schema";

import { resolveReaderScrollPosition } from "./utils/readerScroll";

/**
 * 读者站路由滚动行为接线（spec #79 用户故事 3）。
 *
 * 浏览器「返回」（站内 SPA 返回 / 前进后退）时优先恢复保存的滚动位置；
 * 其它导航回顶。纯判据在 `app/utils/readerScroll.ts`，这里只做接线与类型适配。
 */
export default <RouterConfig>{
  scrollBehavior(_to, _from, savedPosition) {
    return resolveReaderScrollPosition(savedPosition);
  },
};
