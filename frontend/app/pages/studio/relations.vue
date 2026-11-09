<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from "vue";
import { NButton, useMessage } from "naive-ui";
import RelationCard from "~/components/social/RelationCard.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { followUser, unfollowUser, getFansList, getFollowList } from "~/services/follow";
import { useDraftChatJump } from "~/composables/useDraftChatJump";
import { errorMessage } from "~/utils/errorMessage";
import { isFollowing } from "~/utils/relation";

/**
 * 个人中心 - 关注与粉丝（票 327）。
 *
 * 关注与粉丝原本是两个页面（`/studio/follows`、`/studio/fans`），但它们是同一件事
 * 的两面：都是"我和这个人的关系"，只是方向不同；还有一个天然的第三视图「互关」。
 * 三个页面各写一遍列表、分页与动作，是三份会各自漂移的实现。现在合成一页，
 * 用页内顶栏切换，列表本体是同一个卡片组件。
 *
 * 互关没有独立接口：它 ⊆ 关注，所以由关注列表按 `relation === 4` 筛出来。
 * 代价要说清楚——筛完的一页会比服务端一页少，所以互关视图在客户端自己分页
 * （先把关注列表读够，再按本地页长切），不和服务端的 `has_more` 混用。
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
/** 互关一次最多扫这么多个服务端关注页，防止"关注几千人"时无限翻页。 */
const MAX_MUTUAL_SCAN_PAGES = 6;

type RelationTab = "follow" | "fans" | "mutual";
type RelationRow = {
  id: string;
  nickname: string;
  avatar: string;
  abstract: string;
  relation: number;
};

const TABS: Array<{ key: RelationTab; label: string }> = [
  { key: "follow", label: "关注" },
  { key: "fans", label: "粉丝" },
  { key: "mutual", label: "互关" },
];

const message = useMessage();
const { jumpToDraftChat } = useDraftChatJump();
const route = useRoute();
const router = useRouter();

const activeTab = ref<RelationTab>(resolveTab(route.query.tab));
const page = shallowRef(1);
const rows = ref<RelationRow[]>([]);
const hasMore = ref(false);
const pending = ref(true);
const failed = ref(false);
const busy = ref(false);

/** 互关：一次扫到的全部互关行（客户端分页，见文件头注释）。 */
const mutualAll = ref<RelationRow[]>([]);
const mutualExhausted = ref(false);

function resolveTab(value: unknown): RelationTab {
  return value === "fans" || value === "mutual" ? value : "follow";
}

function normalizeFollow(item: { followed_user_id: string; followed_nickname: string; followed_avatar?: string; followed_abstract?: string; relation?: number }): RelationRow {
  return {
    id: item.followed_user_id,
    nickname: item.followed_nickname,
    avatar: item.followed_avatar ?? "",
    abstract: item.followed_abstract ?? "",
    relation: item.relation ?? 0,
  };
}

function normalizeFan(item: { fans_user_id: string; fans_nickname: string; fans_avatar?: string; fans_abstract?: string; relation?: number }): RelationRow {
  return {
    id: item.fans_user_id,
    nickname: item.fans_nickname,
    avatar: item.fans_avatar ?? "",
    abstract: item.fans_abstract ?? "",
    relation: item.relation ?? 0,
  };
}

/** 互关视图：先把关注列表读够，再筛出 `relation === 4`。 */
async function loadMutualPage(target: number) {
  const needed = target * PAGE_SIZE;
  let rawPage = 1;

  while (mutualAll.value.length < needed && !mutualExhausted.value && rawPage <= MAX_MUTUAL_SCAN_PAGES) {
    const result = await getFollowList({ page: rawPage, limit: PAGE_SIZE });
    mutualAll.value = [...mutualAll.value, ...result.list.map(normalizeFollow).filter((row) => row.relation === 4)];
    if (!result.has_more) mutualExhausted.value = true;
    rawPage += 1;
  }

  rows.value = mutualAll.value.slice((target - 1) * PAGE_SIZE, target * PAGE_SIZE);
  hasMore.value =
    mutualAll.value.length > target * PAGE_SIZE ||
    (!mutualExhausted.value && mutualAll.value.length === target * PAGE_SIZE);
}

async function loadPage(target: number) {
  pending.value = true;
  failed.value = false;
  try {
    if (activeTab.value === "mutual") {
      await loadMutualPage(target);
    } else if (activeTab.value === "fans") {
      const result = await getFansList({ page: target, limit: PAGE_SIZE });
      rows.value = result.list.map(normalizeFan);
      hasMore.value = result.has_more;
    } else {
      const result = await getFollowList({ page: target, limit: PAGE_SIZE });
      rows.value = result.list.map(normalizeFollow);
      hasMore.value = result.has_more;
    }
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

/** 切分栏：换一个视图就回到第一页，并且把互关的累积清空重新扫。 */
function switchTab(tab: RelationTab) {
  if (tab === activeTab.value) return;
  activeTab.value = tab;
  const wasOnFirstPage = page.value === 1;
  page.value = 1;
  rows.value = [];
  if (tab === "mutual") {
    mutualAll.value = [];
    mutualExhausted.value = false;
  }
  void router.replace({ query: { ...route.query, tab } });
  // 页码没变（本来就停在第 1 页）时 watch 不会触发，所以这里显式取一次；
  // 页码变了则由 watch 负责，避免同一轮取数发两次。
  if (wasOnFirstPage) void loadPage(1);
}

async function toggleFollow(row: RelationRow) {
  if (busy.value) return;
  busy.value = true;
  try {
    if (isFollowing(row.relation)) {
      await unfollowUser(row.id);
      message.success(row.relation === 4 ? "已取消回关" : "已取消关注");
    } else {
      await followUser(row.id);
      message.success("已回关");
    }
    // 写完以服务端为准：关系是双向的，本地改一条会漏掉对方那侧的计数与互关状态。
    if (activeTab.value === "mutual") {
      mutualAll.value = [];
      mutualExhausted.value = false;
      await loadMutualPage(page.value);
    } else {
      await loadPage(page.value);
    }
  } catch (error) {
    message.error(errorMessage(error, "关注操作失败"));
  } finally {
    busy.value = false;
  }
}

function openMessage(row: RelationRow) {
  // 与作者主页共用同一份跨页契约（四个 draft_* 参数由 buildDraftChatQuery 单一来源生成）。
  void jumpToDraftChat({
    receiverId: row.id,
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
  void loadPage(page.value);
});

watch(page, (value) => {
  void loadPage(value);
});

const summaryText = computed(() => {
  if (pending.value) return "正在读取关系列表…";
  if (failed.value) return "暂时读不到这份关系列表";
  if (!rows.value.length) {
    if (activeTab.value === "mutual") return "还没有互相关注的人";
    if (activeTab.value === "fans") return "还没有粉丝";
    return "还没有关注任何人";
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

      <!-- 页内顶栏：三个视图共用同一个列表与同一个卡片，只换取数来源。 -->
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
        </button>
      </div>

      <div v-if="rows.length" class="relation-card-grid" :data-role="`relation-list-${activeTab}`">
        <RelationCard
          v-for="row in rows"
          :key="row.id"
          :user-id="row.id"
          :nickname="row.nickname"
          :avatar="row.avatar"
          :abstract="row.abstract"
          :relation="row.relation"
          :mode="activeTab"
          :busy="busy"
          @toggle-follow="toggleFollow(row)"
          @message="openMessage(row)"
        />
      </div>

      <ProfileEmptyState
        v-else
        class="mt-5"
        size="md"
        :pending="pending"
        :title="pending ? '正在读取关系列表' : summaryText"
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
