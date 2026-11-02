<script lang="ts">
import { defineComponent, h, resolveComponent } from 'vue'
import type { PropType, VNode } from 'vue'
import type { AdminBreadcrumbItem } from '~/utils/menu'

/**
 * 内容区顶部的面包屑。
 *
 * 只消费 `buildAdminBreadcrumb()` 的数据（祖先链来自菜单拍平视图），组件本身
 * 不推导层级。三条渲染规则与数据结构一一对应：
 *   - 有 path 的祖先 → 可点回退链接；
 *   - 没有 path 的只分组父项 → 纯文本，不产出空 href 链接；
 *   - 当前项 → 当前态（aria-current），不可点。
 *
 * 分隔符只在项与项之间插入，所以单级页面（根页 / 顶层页）天然没有分隔符。
 * NuxtLink 用全局解析而不是 import，单测注册一个 href 桩即可挂载。
 */
export default defineComponent({
  name: 'AdminBreadcrumb',
  props: {
    items: {
      type: Array as PropType<AdminBreadcrumbItem[]>,
      required: true,
    },
  },
  setup(props) {
    const NuxtLink = resolveComponent('NuxtLink')

    function renderCrumb(item: AdminBreadcrumbItem): VNode {
      if (item.current) {
        return h(
          'span',
          { class: 'admin-breadcrumb__current', 'aria-current': 'page' },
          item.label,
        )
      }

      if (item.path) {
        return h(
          NuxtLink,
          { to: item.path, class: 'admin-breadcrumb__link' },
          { default: () => item.label },
        )
      }

      // 只分组、没有落地页的父级：给出层级位置，但不做成链接。
      return h('span', { class: 'admin-breadcrumb__group' }, item.label)
    }

    function renderSeparator(index: number): VNode {
      return h(
        'li',
        {
          key: `separator:${index}`,
          class: 'admin-breadcrumb__item',
          'aria-hidden': 'true',
          'data-breadcrumb-separator': 'true',
        },
        [h('span', { class: 'admin-breadcrumb__sep' }, '/')],
      )
    }

    return () => {
      if (props.items.length === 0) return null

      const children: VNode[] = []
      props.items.forEach((item, index) => {
        if (index > 0) children.push(renderSeparator(index))
        children.push(
          h(
            'li',
            { key: `crumb:${index}:${item.label}`, class: 'admin-breadcrumb__item' },
            [renderCrumb(item)],
          ),
        )
      })

      return h('nav', { class: 'admin-breadcrumb', 'aria-label': '面包屑' }, [
        h('ol', { class: 'admin-breadcrumb__list' }, children),
      ])
    }
  },
})
</script>
