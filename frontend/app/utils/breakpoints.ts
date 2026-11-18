/**
 * 响应式断点的**唯一来源**（票 437 的收口，spec 423 第 1 批）。
 *
 * 为什么单独抽一个文件：CSS 与 JS 都读同一个断点时，以前是「CSS 写 640px、
 * `useHeaderMobilePanel` 再写一遍 `(max-width: 639px)`」——两处各持一份，
 * 改一处就静默分叉。CSS 不能 `import` TS 常量，所以做法是：
 *   1. 数值只在这里定义；
 *   2. JS 侧（`useHeaderMobilePanel`）直接引用本文件；
 *   3. CSS 侧（`AppHeader.vue` 的 `@media (min-width: 640px)` / `(max-width: 639px)`）
 *      在注释里写明来自这里，并由 `test/design/header-breakpoints.test.ts`
 *      断言"CSS 的 640 与 JS 的 640 是同一个数"，分叉立即红。
 *
 * 断点的语义是「**从这一档起算平板**」：640px 及更宽是平板/桌面，639px 及更窄是手机。
 */
export const TABLET_BREAKPOINT_PX = 640;

/** 手机断点的查询串：`(max-width: 639px)` 与 CSS 的 `min-width: 640px` 是同一个分界。 */
export const MOBILE_PANEL_QUERY = `(max-width: ${TABLET_BREAKPOINT_PX - 1}px)`;
