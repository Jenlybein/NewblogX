<script setup lang="ts">
/**
 * 后台头像（决定清单 §6）。
 *
 * 回退顺序：**有图片且能加载** → 显示图片；**能取首字符**（昵称不是邮箱）→ 显示首字符；
 * 否则画人形图标。第三层存在的唯一理由：后台用户列表里大量账号的用户名是邮箱，
 * 取首字符会画出一个毫无意义的 `1`（这正是读者反馈的那个"头像没显示出来"）。
 *
 * 2026-11-13 补第四种情况：**地址有值但图片加载失败**（对象存储里已删除、
 * 域名换了、路径写错）。以前只有"src 为空"才回退，于是列表里出现一个裂开的
 * 破图标 —— 比没有头像更难看。现在加载失败同样落到首字符/图标。
 */
import { computed, ref, watch } from 'vue'
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

/** 图片加载失败。地址一变（换用户 / 重新上传）就复位，否则会一直停在回退态。 */
const broken = ref(false)
watch(url, () => { broken.value = false })

function handleImageError() {
  broken.value = true
}
</script>

<template>
  <NAvatar
    v-if="url && !broken"
    :round="round"
    :size="size"
    :src="url"
    :on-error="handleImageError"
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
