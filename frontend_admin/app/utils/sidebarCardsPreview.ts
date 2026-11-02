import type { SiteSidebarCardItem } from '~/types/api'
import type { SidebarCardDraft } from '~/utils/sidebarCardsAdmin'
import { sidebarCardViews } from '../../../frontend/app/utils/sidebarCards'

/**
 * 后台右栏预览的纯函数层。
 *
 * # 为什么是一条跨包 import
 *
 * 预览不重写渲染规则：它直接调用读者站的 `sidebarCardViews`（同一份纯函数）。
 * 跨包 import 的坑是后台把 `~` 指向后台自己的 `app`，读者站模块内部若再用别名
 * 引用同站文件就会解析到后台（`frontend/app/utils/collection.ts` 已改为同目录相对路径，
 * 那里有注释说明）。这里也刻意用相对路径，让模型的唯一实现在两端是同一份代码。
 *
 * # 预览吃什么
 *
 * 预览渲染的是**本地草稿**，不是回读服务端：标题、顺序、启停、增删都直接由
 * `drafts` 推导。契约里的管理端接口只返回配置、不返回各卡内容，所以新卡/空数据卡
 * 在预览里用一份固定的示例内容撑起版面；示例只影响观感，不进入保存负载。
 */

/**
 * 预览宽度（像素）：等于首页右栏的固定列宽。
 *
 * 真实值来自读者站 `app/assets/css/tailwind.css` 的 `.hero-grid` / `.content-grid`
 * 两栏栅格（`minmax(0,1fr) 310px`）。这里不模拟断点切换，后台永远按这个宽度画。
 * 用例会解析那份样式并断言两边相等，改任何一边都会红。
 */
export const SIDEBAR_PREVIEW_WIDTH_PX = 310

/** 预览示例内容：只用于把卡片版面撑起来，不会提交到服务端。 */
const PREVIEW_SAMPLES: Record<string, Pick<
  SiteSidebarCardItem,
  'hot_articles' | 'latest_comments' | 'tags' | 'collections'
>> = {
  hot_articles: {
    hot_articles: [
      { id: 'preview-hot-1', title: '从零搭建一个 Go 博客', digg_count: 128, view_count: 2048 },
      { id: 'preview-hot-2', title: 'Nuxt 与设计令牌的边界', digg_count: 86, view_count: 1536 },
      { id: 'preview-hot-3', title: '部署笔记：一次可回滚的上线', digg_count: 42, view_count: 900 },
    ],
    latest_comments: [],
    tags: [],
    collections: [],
  },
  latest_comments: {
    hot_articles: [],
    latest_comments: [
      {
        id: 'preview-comment-1',
        content: '把取舍讲得很清楚，尤其是失败路径那段。',
        article_id: 'preview-article-1',
        article_title: '从零搭建一个 Go 博客',
        user_nickname: '读者甲',
        created_at: '2026-11-02T00:00:00Z',
      },
      {
        id: 'preview-comment-2',
        content: '照着做了一遍，能跑通。',
        article_id: 'preview-article-2',
        article_title: '部署笔记：一次可回滚的上线',
        user_nickname: '读者乙',
        created_at: '2026-11-01T00:00:00Z',
      },
    ],
    tags: [],
    collections: [],
  },
  tag_cloud: {
    hot_articles: [],
    latest_comments: [],
    tags: [
      { id: 'preview-tag-1', title: 'Go', article_count: 12 },
      { id: 'preview-tag-2', title: 'Nuxt', article_count: 6 },
      { id: 'preview-tag-3', title: '部署', article_count: 3 },
    ],
    collections: [],
  },
  recommended_collections: {
    hot_articles: [],
    latest_comments: [],
    tags: [],
    collections: [
      {
        id: 'preview-collection-1',
        title: '从零搭一个博客',
        description: '从环境到上线的一次完整记录',
        cover: '',
        article_count: 7,
      },
      {
        id: 'preview-collection-2',
        title: '工程效率',
        description: '',
        cover: '',
        article_count: 0,
      },
    ],
  },
}

/** 该种类的示例内容；未知种类给空数组，让它被渲染模型整张忽略。 */
function previewSample(kind: string): Pick<
  SiteSidebarCardItem,
  'hot_articles' | 'latest_comments' | 'tags' | 'collections'
> {
  const known = PREVIEW_SAMPLES[kind]
  if (known) return known
  return { hot_articles: [], latest_comments: [], tags: [], collections: [] }
}

/**
 * 草稿 → 预览用的读者卡片形状。
 *
 * 只取启用的卡（读者接口也只下发启用项），顺序取草稿下标（与后端整表保存时
 * `sort_order = 下标` 的口径一致）。标题原样传，去空白与默认回退仍由
 * `sidebarCardViews` 统一处理，预览不另立一套标题规则。
 */
export function sidebarCardsPreviewItems(
  drafts: readonly SidebarCardDraft[],
): SiteSidebarCardItem[] {
  return drafts
    .filter(draft => draft.enabled)
    .map((draft, index) => ({
      id: draft.id || `preview-${draft.key}`,
      kind: draft.kind,
      title: draft.title,
      sort_order: index,
      ...previewSample(draft.kind),
    }))
}

/**
 * 预览模型：直接把卡片交给读者站的同一份渲染函数。
 *
 * 这一行是整个票的关键缝：两端口径不是靠约定维持，而是同一份实现。
 * 后台用例拿跨端契约表断言这个出口，若有人在这里换成后台自写的渲染，用例会红。
 */
export function sidebarCardsPreviewViews(cards: readonly SiteSidebarCardItem[]) {
  return sidebarCardViews(cards)
}
