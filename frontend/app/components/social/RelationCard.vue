<script setup lang="ts">
import { computed } from "vue";
import { NDropdown } from "naive-ui";
import { IconDotsVertical } from "@tabler/icons-vue";
import AppAvatar from "~/components/common/AppAvatar.vue";

/**
 * 关系卡片（关注 / 粉丝 / 互关共用）。
 *
 * 形状来自用户给的参照：一个长方形容器里放圆形头像 + 昵称 + 行内按钮，
 * 右侧一个"更多"入口（指针悬浮时出现），菜单里是私信与关系动作。
 *
 * 为什么不用列表行：这三类关系是"一排可扫的对象"，不是文章那种需要阅读的条目。
 * 之前用 naive-ui 的 `NListItem` + `#suffix` 把动作塞在右槽里，而
 * `.n-list-item__suffix` 是 `flex: 0`——中文按钮会被压成一字宽竖排。卡片自己
 * 控制动作区的宽度，没有这个问题。
 *
 * 组件只负责"这颗关系该长什么样、点了该发什么信号"，不取数也不调接口：
 * 页面拿到信号后去调服务，这样三种关系共用一个组件，宽度与文案都不会漂移。
 */
const props = withDefaults(
  defineProps<{
    /** 对方用户 ID（用于跳主页与给调用方的信号）。 */
    userId: string;
    nickname: string;
    avatar?: string;
    abstract?: string;
    /** 关系值：2 我关注了对方 / 3 对方关注了我 / 4 互相关注。 */
    relation?: number;
    /** 这一行是从哪个视图来的：决定按钮与菜单的语义。 */
    mode?: "follow" | "fans" | "mutual";
    /** 请求在途时禁用动作，防连点。 */
    busy?: boolean;
  }>(),
  { avatar: "", abstract: "", relation: 0, mode: "follow", busy: false },
);

const emit = defineEmits<{
  /** 关系动作：关注 / 取关 / 回关。由页面决定具体调哪个接口。 */
  toggleFollow: [];
  /** 发私信。 */
  message: [];
  /** 移除粉丝（单向）。只有"他关注我"的行才会发这个信号。 */
  removeFan: [];
}>();

const mutual = computed(() => props.relation === 4);

/** 主按钮：互关与已关注是状态（灰、不可点），其余是动作。 */
const primary = computed(() => {
  if (mutual.value) return { label: "已互粉", state: true };
  if (props.mode === "follow" && props.relation === 2) return { label: "已关注", state: true };
  if (props.mode === "follow" && props.relation === 3) return { label: "回关", state: false };
  if (props.mode === "fans" && props.relation === 3) return { label: "回关", state: false };
  if (props.mode === "mutual") return { label: "已互粉", state: true };
  return { label: props.mode === "follow" ? "关注" : "回关", state: false };
});

/**
 * 「移除粉丝」只对"他关注我"的行成立（relation 3 或 4），而且不在公开主页出现。
 *
 * 为什么 follow 视图不给：在那里用户的心理模型是"我关注的人"，移除粉丝是另一件事，
 * 混进同一个菜单会让人误以为它会取关。要看粉丝，切到粉丝视图。
 */
const canRemoveFan = computed(() => props.mode !== "follow" && (props.relation === 3 || props.relation === 4));

/**
 * 更多菜单。顺序是"最常用 → 关系动作 → 最重"：
 *   - 发私信：三类关系都能发。
 *   - 取消关注 / 回关：与主按钮互补（主按钮是状态时，这里才是能做的动作）。
 *   - 移除粉丝：单向删除"他关注我"，放最后，因为它是不可撤销的那一个。
 */
const menuItems = computed(() => {
  const items: Array<{ key: string; label: string }> = [{ key: "message", label: "发私信" }];
  if (props.relation === 2 || props.relation === 4) {
    items.push({ key: "toggleFollow", label: "取消关注" });
  } else {
    items.push({ key: "toggleFollow", label: "回关" });
  }
  if (canRemoveFan.value) {
    items.push({ key: "removeFan", label: "移除粉丝" });
  }
  return items;
});

function handleMenu(key: string) {
  if (key === "message") emit("message");
  else if (key === "toggleFollow") emit("toggleFollow");
  else if (key === "removeFan") emit("removeFan");
}
</script>

<template>
  <div class="relation-card" :data-relation="relation" :data-mode="mode">
    <NuxtLink :to="`/users/${userId}`" class="relation-card__avatar" :aria-label="`查看 ${nickname} 的主页`">
      <AppAvatar :size="52" :src="avatar" :name="nickname" fallback="友" />
    </NuxtLink>

    <div class="relation-card__main">
      <NuxtLink :to="`/users/${userId}`" class="relation-card__name">{{ nickname }}</NuxtLink>

      <p v-if="abstract" class="relation-card__abstract">{{ abstract }}</p>

      <button
        type="button"
        class="relation-card__action"
        :class="{ 'is-state': primary.state }"
        :disabled="primary.state || busy"
        :data-role="`relation-action-${userId}`"
        @click="emit('toggleFollow')"
      >
        {{ primary.label }}
      </button>
    </div>

    <!-- 更多入口：指针悬浮/键盘聚焦时出现；触屏没有 hover，所以窄屏常显。 -->
    <NDropdown :options="menuItems" trigger="click" placement="bottom-end" @select="handleMenu">
      <button
        type="button"
        class="relation-card__more"
        :aria-label="`${nickname} 的更多操作`"
        :data-role="`relation-more-${userId}`"
        :disabled="busy"
      >
        <IconDotsVertical :size="16" aria-hidden="true" />
      </button>
    </NDropdown>
  </div>
</template>

<style scoped>
.relation-card {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface-raised);
  transition: border-color var(--motion-fast) var(--motion-ease);
}

.relation-card:hover,
.relation-card:focus-within {
  border-color: var(--color-line-strong);
}

.relation-card__avatar {
  display: block;
  flex: 0 0 auto;
  border-radius: var(--radius-pill);
}

.relation-card__main {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}

.relation-card__name {
  max-width: 100%;
  overflow: hidden;
  color: var(--color-ink);
  font-size: 0.9375rem;
  font-weight: 600;
  line-height: 1.35;
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.relation-card__name:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}

.relation-card__abstract {
  margin: 0;
  overflow: hidden;
  max-width: 100%;
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
  line-height: 1.5;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

/* 动作按钮：互关/已关注是"状态"（灰底、不可点），其余是"动作"（描边主色）。 */
.relation-card__action {
  padding: 5px 16px;
  border: 1px solid var(--color-line-strong);
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--color-ink);
  font-size: 0.8125rem;
  font-weight: 600;
  line-height: 1.4;
  cursor: pointer;
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    background-color var(--motion-fast) var(--motion-ease);
}

.relation-card__action:hover:not(:disabled) {
  background: var(--color-line-faint);
}

.relation-card__action.is-state,
.relation-card__action:disabled {
  border-color: transparent;
  background: var(--color-line-faint);
  color: var(--color-ink-muted);
  cursor: default;
}

.relation-card__action:focus-visible,
.relation-card__more:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

/* 更多入口默认隐身（占位不占宽），悬浮或焦点进入时显形。 */
.relation-card__more {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--color-ink-muted);
  cursor: pointer;
  opacity: 0;
  transition:
    opacity var(--motion-fast) var(--motion-ease),
    border-color var(--motion-fast) var(--motion-ease);
}

.relation-card:hover .relation-card__more,
.relation-card:focus-within .relation-card__more {
  opacity: 1;
}

.relation-card__more:hover {
  border-color: var(--color-line);
  color: var(--color-ink);
}

/* 触屏没有 hover：更多入口常显，否则菜单永远打不开。 */
@media (hover: none) {
  .relation-card__more {
    opacity: 1;
  }
}
</style>
