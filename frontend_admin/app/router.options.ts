import type { RouterConfig } from '@nuxt/schema'

/**
 * 后台的路由滚动策略（票 481）。
 *
 * 后台外壳是"左栏 + 内容区"各自滚动的固定一屏布局（见 assets/css/admin.css 的
 * `.admin-shell`）：窗口本身不滚，所以**窗口级的滚动复位在这里没有意义**，
 * 而 Nuxt 的默认 scrollBehavior 恰恰只做那件事 —— 顺手把左栏也带回了顶部。
 *
 * 这个函数把"切页"的语义收敛成一句：只把**内容区**滚回顶部，左栏保持用户
 * 自己的位置。浏览器的前进/后退（popstate）也给同一份答案：后台没有"上次看到
 * 的第几屏"这种可恢复的东西，回到顶部比恢复一个错位的内容区更可预期。
 *
 * 找不到容器时返回 `false`：交给浏览器默认行为，而不是返回一个会抛错的 target
 * （例如登录页 / setup 页没有 AdminShell，它们本来就该按窗口滚）。
 */
export default <RouterConfig>{
  scrollBehavior() {
    if (!import.meta.client) return false
    const target = document.getElementById('admin-content-scroll')
    if (target === null) return false
    return { el: '#admin-content-scroll', top: 0, left: 0, behavior: 'instant' as const }
  },
}
