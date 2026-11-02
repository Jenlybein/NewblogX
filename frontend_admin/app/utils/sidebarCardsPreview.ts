import type { SiteSidebarCardItem } from '~/types/api'
import type { SidebarCardDraft } from '~/utils/sidebarCardsAdmin'
import { sidebarCardViews } from '~/utils/sidebarCardViews'

/**
 * 后台右栏预览的纯函数层。
 *
 * # 为什么用后台自己的渲染模型，而不是 import 读者站那份
 *
 * `deploy/docker/admin/Dockerfile` 写着「frontend_admin 为自洽应用」：镜像只
 * `COPY frontend_admin`，构建上下文里没有 `../frontend`。运行时跨包 import 读者站模块
 * 在生产构建里必然失败（本地两个目录都在，看不出来）。
 *
 * 口径一致性由**共享契约表**保证：读者站的跨端契约表夹具
 * 被两端用例一起消费，任一端改了渲染规则自己那侧就红。见 `sidebarCardViews.ts` 的说明。
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
      // 富文本卡用草稿里正在编辑的正文，其它种类没有正文。
      content: draft.kind === 'rich_text' ? draft.content : '',
      ...previewSample(draft.kind),
    }))
}

/**
 * 预览模型：把卡片交给后台自己的同口径渲染函数。
 *
 * 与读者站的一致性是**契约表**保证的（两端用例都对着
 * 读者站的跨端契约表夹具 断言），不是靠共用运行时模块——
 * 后者会让后台镜像不自洽。
 */
export function sidebarCardsPreviewViews(cards: readonly SiteSidebarCardItem[]) {
  return sidebarCardViews(cards)
}
