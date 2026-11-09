<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from "vue";
import { NButton, useDialog, useMessage } from "naive-ui";
import RelationCard from "~/components/social/RelationCard.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import {
  followUser,
  getRelationCounts,
  getRelations,
  removeFan as removeFanRequest,
  unfollowUser,
  type RelationKind,
} from "~/services/follow";
import { useDraftChatJump } from "~/composables/useDraftChatJump";
import { errorMessage } from "~/utils/errorMessage";
import { isFollowing } from "~/utils/relation";
import type { RelationCounts, RelationItem } from "~/types/api";

/**
 * 个人中心 - 关注与粉丝（票 327）。
 *
 * 关注与粉丝是同一件事的两面（方向不同），互关是两者的交集，所以三种视图合在
 * 一页，用页内顶栏切换；列表本体是同一个卡片。
 *
 * 取数只有一条路径：`GET /api/relations?type=...`。互关由**服务端**先筛后分页，
 * 所以这里没有"筛完不够一页还要往后扫"的逻辑——那是本票之前客户端筛互关的
 * 临时办法，`has_more` 与每页条数都不可信。顶栏的数字来自
 * `GET /api/relations/counts`（与列表同一套可见性判据）。
 *
 * 写动作两类：`/api/follow/{id}`（我对别人的关注）与
 * `/api/relations/fans/{id}`（单方面移除关注我的人）。
 */
definePageMeta({
  layout: "studio",
  middleware: "auth",
});

useSeoMeta({
  title: "个人中心 - 关注与粉丝",
});
useNoIndexPage();

const PAGE_SIZE = 24;

type RelationTab = RelationKind;

const TABS: Array<{ key: RelationTab; label: string }> = [
  { key: "follow", label: "关注" },
  { key: "fans", label: "粉丝" },
  { key: "mutual", label: "互关" },
];

const message = useMessage();
const dialog = useDialog();
const { jumpToDraftChat } = useDraftChatJump();
const route = useRoute();
const router = useRouter();

const activeTab = ref<RelationTab>(resolveTab(route.query.tab));
const page = shallowRef(1);
const rows = ref<RelationItem[]>([]);
const hasMore = ref(false);
const pending = ref(true);
const failed = ref(false);
const busy = ref(false);
/**
 * 顶栏计数。读失败就留空——徽标少一个数字不影响列表可用，
 * 但它绝不能变成"0"（那会让人以为关系没了）。
 */
const counts = shallowRef<RelationCounts | null>(null);

function resolveTab(value: unknown): RelationTab {
  return value === "fans" || value === "mutual" ? value : "follow";
}

function tabCount(kind: RelationTab): number | null {
  if (!counts.value) return null;
  return counts.value[kind] ?? null;
}

async function loadPage(target: number) {
  pending.value = true;
  failed.value = false;
  try {
    const result = await getRelations({ type: activeTab.value, page: target, limit: PAGE_SIZE });
    rows.value = result.list;
    hasMore.value = result.has_more;
  } catch (error) {
    // 失败态与空态必须分开：失败时落进"还没有人"会让用户以为关系没了。
    failed.value = true;
    rows.value = [];
    hasMore.value = false;
    message.error(errorMessage(error, "关系列表加载失败"));
  } finally {
    pending.value = false;
  }
}

async function loadCounts() {
  try {
    counts.value = await getRelationCounts();
  } catch {
    counts.value = null;
  }
}

/**
 * 写入之后以服务端为准。
 *
 * 关系是双向的：关注一个人会同时改两个视图的计数，也会让他从"粉丝"变成"互关"。
 * 本地改一行就会漏掉这些联动，所以一律重拉列表与计数。
 *
 * 顺带处理一个边界：移除/取关删掉本页最后一条时退回上一页，
 * 否则会停在一张空页上（`page` 变化会触发 watch 重新取数）。
 */
async function reloadAfterMutation() {
  await Promise.all([loadPage(page.value), loadCounts()]);
  if (!rows.value.length && page.value > 1) {
    page.value -= 1;
  }
}

/** 切分栏：换一个视图就回到第一页。 */
function switchTab(tab: RelationTab) {
  if (tab === activeTab.value) return;
  activeTab.value = tab;
  const wasOnFirstPage = page.value === 1;
  page.value = 1;
  rows.value = [];
  void router.replace({ query: { ...route.query, tab } });
  // 页码没变（本来就停在第 1 页）时 watch 不会触发，所以这里显式取一次。
  if (wasOnFirstPage) void loadPage(1);
}

async function toggleFollow(row: RelationItem) {
  if (busy.value) return;
  busy.value = true;
  try {
    if (isFollowing(row.relation)) {
      await unfollowUser(row.user_id);
      message.success("已取消关注");
    } else {
      await followUser(row.user_id);
      message.success(row.relation === 3 ? "已回关" : "已关注");
    }
    await reloadAfterMutation();
  } catch (error) {
    message.error(errorMessage(error, "关注操作失败"));
  } finally {
    busy.value = false;
  }
}

/** 移除粉丝是不可撤销的，先问一次；讲清楚是单向的、对方不会收到通知。 */
function confirmRemoveFan(row: RelationItem) {
  dialog.warning({
    title: "移除粉丝",
    content: `移除后「${row.nickname}」将不再关注你。对方不会收到通知，之后仍可再次关注。`,
    positiveText: "移除",
    negativeText: "取消",
    onPositiveClick: () => removeFanById(row),
  });
}

async function removeFanById(row: RelationItem) {
  if (busy.value) return;
  busy.value = true;
  try {
    await removeFanRequest(row.user_id);
    message.success("已移除该粉丝");
    await reloadAfterMutation();
  } catch (error) {
    message.error(errorMessage(error, "移除粉丝失败"));
  } finally {
    busy.value = false;
  }
}

function openMessage(row: RelationItem) {
  // 与作者主页共用同一份跨页契约（四个 draft_* 参数由 buildDraftChatQuery 单一来源生成）。
  void jumpToDraftChat({
    receiverId: row.user_id,
    receiverNickname: row.nickname || "新会话",
    receiverAvatar: row.avatar || "",
    relation: row.relation,
  });
}

function previousPage() {
  if (page.value <= 1) return;
  page.value -= 1;
}

function nextPage() {
  if (!hasMore.value) return;
  page.value += 1;
}

onMounted(() => {
  void loadCounts();
  void loadPage(page.value);
});

watch(page, (value) => {
  void loadPage(value);
});

/**
 * 空态的标题只说"没有"。
 *
 * 不复用 `summaryText`：徽标可能已经读到 12 人（例如刚移除掉本页最后一条、
 * 计数还没回来），而列表此刻是空的——那时标题写"关注 12 人"会让空态自相矛盾。
 */
const emptyTitle = computed(() => {
  if (activeTab.value === "mutual") return "还没有互相关注的人";
  if (activeTab.value === "fans") return "还没有粉丝";
  return "还没有关注任何人";
});

const summaryText = computed(() => {
  if (pending.value) return "正在读取关系列表…";
  if (failed.value) return "暂时读不到这份关系列表";
  if (!rows.value.length) return emptyTitle.value;
  const total = tabCount(activeTab.value);
  if (total !== null) {
    const noun = activeTab.value === "fans" ? "粉丝" : activeTab.value === "mutual" ? "互关" : "关注";
    return `${noun} ${total} 人`;
  }
  return `共 ${rows.value.length} 人${hasMore.value ? "（还有更多）" : ""}`;
});

const emptyDescription = computed(() => {
  if (failed.value) return "网络或服务波动，你的关系数据不会丢。稍后重试即可。";
  if (activeTab.value === "mutual") return "当你和对方互相关注时，这里会列出彼此。";
  if (activeTab.value === "fans") return "继续发布内容与参与互动后，这里会逐渐出现关注你的人。";
  return "可以从作者主页或文章详情页开始关注感兴趣的创作者。";
});
</script>

<template>
  <div class="page-stack">
    <section class="studio-list-card">
      <div class="studio-toolbar studio-toolbar--stack">
        <div class="min-w-0">
          <div class="eyebrow">Relations</div>
          <h1 class="section-title mt-1">关注与粉丝</h1>
          <p class="muted mt-1 text-sm">{{ summaryText }}</p>
        </div>
      </div>

      <!-- 页内顶栏：三个视图共用同一个卡片与同一份分页，只换取数参数。 -->
      <div class="relation-tabs" role="tablist" aria-label="关系视图">
        <button
          v-for="tab in TABS"
          :key="tab.key"
          type="button"
          role="tab"
          class="relation-tabs__item"
          :class="{ 'is-active': activeTab === tab.key }"
          :aria-selected="activeTab === tab.key"
          :data-role="`relation-tab-${tab.key}`"
          @click="switchTab(tab.key)"
        >
          {{ tab.label }}
          <span v-if="tabCount(tab.key) !== null" class="relation-tabs__count">{{ tabCount(tab.key) }}</span>
        </button>
      </div>

      <div v-if="rows.length" class="relation-card-grid" :data-role="`relation-list-${activeTab}`">
        <RelationCard
          v-for="row in rows"
          :key="row.user_id"
          :user-id="row.user_id"
          :nickname="row.nickname"
          :avatar="row.avatar"
          :abstract="row.abstract"
          :relation="row.relation"
          :mode="activeTab"
          :busy="busy"
          @toggle-follow="toggleFollow(row)"
          @message="openMessage(row)"
          @remove-fan="confirmRemoveFan(row)"
        />
      </div>

      <ProfileEmptyState
        v-else
        class="mt-5"
        size="md"
        :pending="pending"
        :title="pending ? '正在读取关系列表' : failed ? '暂时读不到这份关系列表' : emptyTitle"
        :description="pending ? '稍候会自动出现。' : emptyDescription"
      />

      <div v-if="rows.length" class="relation-pager">
        <span>第 {{ page }} 页</span>
        <div class="flex items-center gap-3">
          <NButton
            quaternary
            size="small"
            data-role="relation-prev"
            :disabled="page <= 1 || pending"
            @click="previousPage()"
          >
            上一页
          </NButton>
          <NButton
            quaternary
            size="small"
            data-role="relation-next"
            :disabled="!hasMore || pending"
            @click="nextPage()"
          >
            下一页
          </NButton>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* 顶栏切换：三个等宽小胶囊，选中态用语义 token（不写裸色值）。 */
.relation-tabs {
  display: inline-flex;
  gap: 4px;
  margin-top: 16px;
  padding: 4px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-pill);
  background: var(--color-line-faint);
}

.relation-tabs__item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 18px;
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
  font-weight: 600;
  cursor: pointer;
  transition:
    background-color var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease);
}

.relation-tabs__item:hover {
  color: var(--color-ink);
}

.relation-tabs__item.is-active {
  background: var(--color-surface-raised);
  color: var(--color-ink);
}

.relation-tabs__item:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

/* 计数徽标：等宽数字，随选中态轻微加深，不做独立的彩色强调。 */
.relation-tabs__count {
  color: var(--color-ink-muted);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  font-weight: 500;
}

.relation-tabs__item.is-active .relation-tabs__count {
  color: var(--color-ink);
}

/* 卡片栅格：个人中心的窄栏里两列，宽一些时三列。 */
.relation-card-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  margin-top: 16px;
}

@media (min-width: 640px) {
  .relation-card-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1100px) {
  .relation-card-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

.relation-pager {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 20px;
  color: var(--color-ink-muted);
  font-size: 0.875rem;
}
</style>
