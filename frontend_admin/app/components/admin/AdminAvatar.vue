<script setup lang="ts">
/**
 * 后台头像（决定清单 §6）。
 *
 * 三层回退：**有图片地址** → 显示图片；**能取首字符**（昵称不是邮箱）→ 显示首字符；
 * 否则画人形图标。第三层存在的唯一理由：后台用户列表里大量账号的用户名是邮箱，
 * 取首字符会画出一个毫无意义的 `1`（这正是读者反馈的那个"头像没显示出来"）。
 */
import { computed } from 'vue'
import { NAvatar } from 'naive-ui'
import { IconUser } from '@tabler/icons-vue'
import { resolveAdminAvatarInitial, resolveAdminAvatarUrl } from '~/utils/adminAvatar'

const props = withDefaults(defineProps<{
  /** 头像地址（可能是相对路径或已被补全的全地址）。 */
  src?: string
  /** 展示名：昵称优先，没有昵称再传用户名。 */
  name?: string
  /** 头像边长（px）。 */
  size?: number
  round?: boolean
}>(), {
  src: '',
  name: '',
  size: 36,
  round: true,
})

const url = computed(() => resolveAdminAvatarUrl(props.src))
const initial = computed(() => resolveAdminAvatarInitial(props.name))
const iconSize = computed(() => Math.max(14, Math.round(props.size * 0.5)))
</script>

<template>
  <NAvatar
    v-if="url"
    :round="round"
    :size="size"
    :src="url"
    data-role="admin-avatar-image" />
  <NAvatar
    v-else-if="initial"
    :round="round"
    :size="size"
    class="admin-avatar--initial"
    data-role="admin-avatar-initial">{{ initial }}</NAvatar>
  <NAvatar
    v-else
    :round="round"
    :size="size"
    class="admin-avatar--icon"
    data-role="admin-avatar-icon">
    <IconUser :size="iconSize" aria-hidden="true" />
  </NAvatar>
</template>
