<script lang="ts">
import { defineComponent, h, resolveComponent } from 'vue'
import type { Component, PropType, VNode } from 'vue'
import {
  IconAlertTriangle,
  IconArticle,
  IconBell,
  IconChartBar,
  IconChevronDown,
  IconDatabase,
  IconPhoto,
  IconPin,
  IconSettings,
  IconShieldCheck,
  IconTag,
  IconUsers,
} from '@tabler/icons-vue'
import type { AdminNavIcon, AdminNavMenuEntry } from '~/utils/menu'

// 图标在这里映射；菜单结构在 utils/menu.ts（唯一来源，另有单测对账页面文件）。
const ICONS: Record<AdminNavIcon, Component> = {
  chart: IconChartBar,
  shield: IconShieldCheck,
  article: IconArticle,
  pin: IconPin,
  tag: IconTag,
  users: IconUsers,
  settings: IconSettings,
  photo: IconPhoto,
  bell: IconBell,
  database: IconDatabase,
  alert: IconAlertTriangle,
}

/**
 * 侧栏两级菜单。
 *
 * 只消费 `buildAdminNavMenu()` 的渲染视图：分组标题、子项链接、展开态与高亮态
 * 都是数据，组件不自己推导。父项没有 path 时标题渲染成文本而不是链接（不跳空页）。
 *
 * NuxtLink 用全局解析而不是 import：这样组件能脱离 Nuxt 在单测里挂载，
 * 单测注册一个 href 桩即可表达"用户点了哪个链接"。
 */
export default defineComponent({
  name: 'AdminNavTree',
  props: {
    entries: {
      type: Array as PropType<AdminNavMenuEntry[]>,
      required: true,
    },
  },
  emits: {
    toggle: (key: string) => typeof key === 'string',
  },
  setup(props, { emit }) {
    const NuxtLink = resolveComponent('NuxtLink')

    function renderIcon(entry: AdminNavMenuEntry): VNode {
      return h(ICONS[entry.icon], {
        size: 18,
        strokeWidth: 1.5,
        class: 'admin-nav__icon',
      })
    }

    function renderLabel(entry: AdminNavMenuEntry): VNode[] {
      return [renderIcon(entry), h('span', { class: 'admin-nav__label' }, entry.label)]
    }

    function renderEntry(entry: AdminNavMenuEntry): VNode {
      if (entry.kind === 'link') {
        return h(
          'li',
          { key: entry.key, class: 'admin-nav__item', 'data-depth': entry.depth },
          [
            h(
              NuxtLink,
              {
                to: entry.path,
                class: ['admin-nav__link', { 'admin-nav__link--active': entry.active }],
                'aria-current': entry.active ? 'page' : undefined,
              },
              { default: () => renderLabel(entry) },
            ),
          ],
        )
      }

      // 父项永远是"展开/收起"的开关，不是链接：
      // 它有自身页面时那页会作为第一个子项出现（见 utils/menu.ts 的树定义），
      // 点标题不会把人带去另一个页面。
      const header = h(
        'button',
        {
          type: 'button',
          class: 'admin-nav__group-title',
          'aria-expanded': entry.expanded ? 'true' : 'false',
          onClick: () => emit('toggle', entry.key),
        },
        renderLabel(entry),
      )

      return h(
        'li',
        { key: entry.key, class: 'admin-nav__item', 'data-depth': entry.depth },
        [
          h(
            'div',
            {
              class: ['admin-nav__group', { 'admin-nav__group--active': entry.active }],
            },
            [
              header,
              h(
                'button',
                {
                  type: 'button',
                  class: 'admin-nav__toggle',
                  'aria-expanded': entry.expanded ? 'true' : 'false',
                  'aria-label': `${entry.expanded ? '收起' : '展开'}${entry.label}`,
                  onClick: () => emit('toggle', entry.key),
                },
                [
                  h(IconChevronDown, {
                    size: 16,
                    strokeWidth: 1.5,
                    class: [
                      'admin-nav__chevron',
                      { 'admin-nav__chevron--open': entry.expanded },
                    ],
                  }),
                ],
              ),
            ],
          ),
          entry.expanded && entry.children.length > 0
            ? h('ul', { class: 'admin-nav__children' }, entry.children.map(renderEntry))
            : null,
        ],
      )
    }

    return () => h('ul', { class: 'admin-nav' }, props.entries.map(renderEntry))
  },
})
</script>
