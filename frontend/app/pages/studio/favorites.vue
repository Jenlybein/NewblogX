<script setup lang="ts">
/**
 * 个人中心 · 收藏夹（票 345 改版）。
 *
 * 版式从"左栏分组列表 + 右栏内容"换成"收藏夹网格 + 大弹窗"：
 *  - 一进来先看全部收藏夹（名称、简介、篇数），点哪张进哪张；
 *  - 收藏夹的资料编辑/删除收进卡片 hover 与弹窗里，不再占一整条工具栏；
 *  - 夹子内容在大弹窗里以多列文章卡呈现，滚动到底自动加载下一页。
 *
 * 三条交互语义（都被用户踩过，别再退化）：
 *  1. 新建收藏夹**必须填资料**：复用同一个弹窗（无标题、按钮「确定」），
 *     不再一点就建一个叫「新的收藏夹」的占位夹；
 *  2. 新建/编辑后留在网格里，新卡高亮一下 —— 否则"建完还是空"像是没生效；
 *  3. 删除只删分组与其下收藏关系，文章本身不动（后端已按此实现）。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { NButton, NInput, NModal, NSkeleton, useMessage } from 'naive-ui'
import ArticleGridCard from '~/components/article/ArticleGridCard.vue'
import {
  createFavoriteFolder,
  deleteFavoriteFolders,
  getFavoriteFolderArticles,
  getOwnFavoriteFolders,
  removeFavoriteFolderArticles,
  updateFavoriteFolder,
} from '~/services/favorite'
import type { FavoriteArticleItem } from '~/types/api'
import { errorMessage } from '~/utils/errorMessage'

definePageMeta({
  layout: 'studio',
  middleware: 'auth',
})

/** 弹窗内每页条数：2–3 列下正好两整排（票 345 决定）。 */
const PAGE_SIZE = 24

const message = useMessage()

const { data: folders, pending: foldersPending, refresh: refreshFolders } = await useAsyncData(
  'studio-favorite-folders',
  () => getOwnFavoriteFolders(),
)

/** 当前打开的大弹窗对应的收藏夹 id；空串表示关闭。 */
const openFolderId = shallowRef('')
const openFolder = computed(() => folders.value?.list.find(item => item.id === openFolderId.value))

const articles = ref<FavoriteArticleItem[]>([])
const articlesLoading = ref(false)
const page = shallowRef(1)
const hasMore = shallowRef(false)

async function loadArticles(reset = false) {
  if (!openFolderId.value) return
  if (reset) {
    page.value = 1
    articles.value = []
  }
  articlesLoading.value = true
  try {
    const data = await getFavoriteFolderArticles({
      favoriteId: openFolderId.value,
      page: page.value,
      limit: PAGE_SIZE,
      // 本人读自己的夹子必须带 token（票 344）。
      auth: true,
    })
    const list = data?.list ?? []
    articles.value = reset ? list : [...articles.value, ...list]
    hasMore.value = Boolean(data?.has_more)
  } catch (error) {
    message.error(errorMessage(error, '加载收藏内容失败'))
  } finally {
    articlesLoading.value = false
  }
}

function openFolderModal(id: string) {
  openFolderId.value = id
  void loadArticles(true)
}

function closeFolderModal() {
  openFolderId.value = ''
  articles.value = []
  page.value = 1
  hasMore.value = false
}

async function loadMore() {
  if (articlesLoading.value || !hasMore.value) return
  page.value += 1
  await loadArticles(false)
}

/** 滚动到底：弹窗内滚动加载（不翻页）。 */
function onModalScroll(event: Event) {
  const el = event.target as HTMLElement
  if (!el) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 64) void loadMore()
}

async function removeArticle(articleId: string) {
  if (!openFolderId.value) return
  try {
    await removeFavoriteFolderArticles({ favoriteId: openFolderId.value, articleIds: [articleId] })
    message.success('文章已移出收藏夹')
    await loadArticles(true)
    await refreshFolders()
  } catch (error) {
    message.error(errorMessage(error, '移除收藏内容失败'))
  }
}

// ── 收藏夹资料弹窗（新建与编辑共用同一个）─────────────────────────

const editorOpen = ref(false)
const editingId = shallowRef('')
const saving = ref(false)
const form = ref({ title: '', abstract: '' })

/** 名称上限 32 与库里的 varchar(32) 对齐；简介 120 是产品上限。 */
const TITLE_MAX = 32
const ABSTRACT_MAX = 120

const isEditing = computed(() => Boolean(editingId.value))
const titleError = computed(() => {
  const value = form.value.title.trim()
  if (!value) return '请填写收藏夹名称'
  if (value.length > TITLE_MAX) return `名称不能超过 ${TITLE_MAX} 个字`
  return ''
})

function openCreateEditor() {
  editingId.value = ''
  form.value = { title: '', abstract: '' }
  editorOpen.value = true
}

function openEditEditor(id: string) {
  const folder = folders.value?.list.find(item => item.id === id)
  if (!folder) return
  editingId.value = id
  form.value = { title: folder.title ?? '', abstract: folder.abstract ?? '' }
  editorOpen.value = true
}

/** 新建后高亮的那张卡：让"刚建好的夹子"在网格里一眼可见。 */
const highlightId = shallowRef('')
let highlightTimer: ReturnType<typeof setTimeout> | null = null

function flashHighlight(id: string) {
  highlightId.value = id
  if (highlightTimer) clearTimeout(highlightTimer)
  highlightTimer = setTimeout(() => {
    highlightId.value = ''
    highlightTimer = null
  }, 2400)
}

onBeforeUnmount(() => {
  if (highlightTimer) clearTimeout(highlightTimer)
})

async function submitEditor() {
  if (titleError.value) return
  saving.value = true
  try {
    const payload = { title: form.value.title.trim(), abstract: form.value.abstract.trim() }
    if (isEditing.value) {
      await updateFavoriteFolder({ id: editingId.value, ...payload })
      message.success('收藏夹已更新')
    } else {
      await createFavoriteFolder(payload)
      message.success('收藏夹已创建')
    }
    editorOpen.value = false
    await refreshFolders()
    if (!isEditing.value) {
      // 后端不返回新夹子的 id：按标题找回"刚建的那个"，找到就高亮。
      const created = folders.value?.list.find(item => item.title === payload.title)
      if (created) {
        await nextTick()
        flashHighlight(created.id)
      }
    }
  } catch (error) {
    // 「名称重复」等可读文案由后端给出，原样展示。
    message.error(errorMessage(error, '保存收藏夹失败'))
  } finally {
    saving.value = false
  }
}

const deleteTarget = shallowRef('')

function askDelete(id: string) {
  deleteTarget.value = id
}

async function confirmDelete() {
  const id = deleteTarget.value
  if (!id) return
  try {
    await deleteFavoriteFolders([id])
    message.success('收藏夹已删除')
    deleteTarget.value = ''
    if (openFolderId.value === id) closeFolderModal()
    await refreshFolders()
  } catch (error) {
    message.error(errorMessage(error, '删除收藏夹失败'))
  }
}

const deleteFolderTitle = computed(
  () => folders.value?.list.find(item => item.id === deleteTarget.value)?.title ?? '')

watch(folders, (value) => {
  if (!value?.list.length) closeFolderModal()
})

useSeoMeta({ title: '个人中心 - 收藏夹' })
useNoIndexPage()
</script>

<template>
  <div class="page-stack">
    <section class="surface-card p-5 md:p-6">
      <div class="studio-toolbar">
        <div>
          <h2 class="section-title">收藏夹</h2>
          <p class="muted">共 {{ folders?.list.length ?? 0 }} 个收藏夹，点开查看里面的文章。</p>
        </div>
        <NButton type="primary" data-role="folder-create" @click="openCreateEditor()">新建收藏夹</NButton>
      </div>

      <div v-if="foldersPending" class="folder-grid mt-5">
        <div v-for="idx in 4" :key="`folder-skeleton-${idx}`" class="folder-card">
          <NSkeleton text width="60%" />
          <NSkeleton text width="90%" class="mt-3" />
        </div>
      </div>

      <div v-else-if="folders?.list.length" class="folder-grid mt-5" data-role="folder-grid">
        <article
          v-for="folder in folders.list"
          :key="folder.id"
          class="folder-card"
          :class="{ 'folder-card--highlight': highlightId === folder.id }"
          :data-role="`folder-card-${folder.id}`">
          <button type="button" class="folder-card__open" @click="openFolderModal(folder.id)">
            <header class="folder-card__head">
              <strong class="folder-card__title">{{ folder.title }}</strong>
              <span class="folder-card__count" data-role="folder-count">{{ folder.article_count ?? 0 }} 篇</span>
            </header>
            <p class="folder-card__abstract">{{ folder.abstract || '还没有填写简介' }}</p>
          </button>

          <div class="folder-card__actions">
            <NButton size="tiny" tertiary data-role="folder-edit" @click.stop="openEditEditor(folder.id)">编辑</NButton>
            <NButton size="tiny" tertiary type="error" data-role="folder-delete" @click.stop="askDelete(folder.id)">
              删除
            </NButton>
          </div>
        </article>
      </div>

      <StudioEmptyState
        v-else
        title="还没有收藏夹"
        description="点右上角「新建收藏夹」建第一个，之后在文章页就能把内容收进来。"
        class="mt-5" />
    </section>

    <!-- 夹子内容：大弹窗 + 多列文章卡 -->
    <NModal
      :show="Boolean(openFolderId)"
      preset="card"
      :bordered="false"
      class="favorite-modal"
      style="width: min(1100px, calc(100vw - 32px)); max-height: 85vh"
      @update:show="(value: boolean) => { if (!value) closeFolderModal() }">
      <template #header>
        <div class="favorite-modal__head">
          <h2 class="section-title">{{ openFolder?.title || '收藏夹' }}</h2>
          <span class="muted">{{ openFolder?.article_count ?? 0 }} 篇 · 滚动到底自动加载</span>
        </div>
      </template>

      <div v-if="openFolderId" class="favorite-modal__body" data-role="folder-articles" @scroll="onModalScroll">
        <div v-if="articles.length" class="article-grid">
          <ArticleGridCard
            v-for="item in articles"
            :key="item.article_id"
            :title="item.title"
            :cover="item.cover"
            :date="item.updated_at || item.favorited_at"
            :to="`/article/${item.article_id}`"
            :view-count="item.view_count"
            :digg-count="item.digg_count"
            :comment-count="item.comment_count"
            :favor-count="item.favor_count"
            action-label="移出收藏夹"
            @action="removeArticle(item.article_id)" />
        </div>

        <div v-else-if="articlesLoading" class="article-grid">
          <article v-for="idx in 6" :key="`article-skeleton-${idx}`" class="article-grid-card">
            <NSkeleton class="article-grid-card__cover" />
            <NSkeleton text width="80%" class="mt-3" />
            <NSkeleton text width="55%" class="mt-2" />
          </article>
        </div>

        <StudioEmptyState
          v-else
          title="这个收藏夹还是空的"
          description="先去文章详情页收藏内容，或者换一个收藏夹继续整理。" />
      </div>
    </NModal>

    <!-- 收藏夹资料：新建与编辑共用，无标题条 -->
    <NModal
      v-model:show="editorOpen"
      preset="card"
      :bordered="false"
      class="favorite-editor"
      style="width: min(560px, calc(100vw - 32px))"
      :mask-closable="false">
      <div class="grid gap-4">
        <label class="space-y-2">
          <span class="text-sm font-medium">收藏夹名称</span>
          <NInput
            v-model:value="form.title"
            :maxlength="TITLE_MAX"
            show-count
            placeholder="例如：Vue3 精读"
            data-role="folder-editor-title" />
          <span v-if="titleError" class="text-xs text-danger">{{ titleError }}</span>
        </label>
        <label class="space-y-2">
          <span class="text-sm font-medium">收藏夹简介</span>
          <NInput
            v-model:value="form.abstract"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 5 }"
            :maxlength="ABSTRACT_MAX"
            show-count
            placeholder="一句话说明这个夹子收什么"
            data-role="folder-editor-abstract" />
        </label>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton quaternary data-role="folder-editor-cancel" @click="editorOpen = false">取消</NButton>
          <NButton
            type="primary"
            :loading="saving"
            :disabled="Boolean(titleError)"
            data-role="folder-editor-submit"
            @click="submitEditor()">
            确定
          </NButton>
        </div>
      </template>
    </NModal>

    <NModal
      :show="Boolean(deleteTarget)"
      preset="card"
      :bordered="false"
      style="width: min(480px, calc(100vw - 32px))"
      @update:show="(value: boolean) => { if (!value) deleteTarget = '' }">
      <p>确认删除收藏夹「{{ deleteFolderTitle }}」？</p>
      <p class="muted mt-2 text-sm">只删这个分组与其中的收藏关系，文章本身不会被删除。</p>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton quaternary data-role="folder-delete-cancel" @click="deleteTarget = ''">取消</NButton>
          <NButton type="error" data-role="folder-delete-confirm" @click="confirmDelete()">删除</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
