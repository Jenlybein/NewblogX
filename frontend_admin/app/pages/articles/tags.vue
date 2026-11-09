<script setup lang="ts">
/**
 * 标签管理页（票 322）：挂在「文章管理」下。
 *
 * 页面本身只交代这页的语义边界，列表 / 编辑 / 启停 / 删除都在
 * `TagManager` 里 —— 与合集页「页面薄、组件厚」的分工一致。
 */
import { NSpace } from 'naive-ui'
import TagManager from '~/components/articles/TagManager.vue'
</script>

<template>
  <NSpace vertical :size="16">
    <div class="admin-page__head">
      <div>
        <h1 class="admin-page__title">标签管理</h1>
        <p class="admin-hint">
          标签是全站词库：发布页的标签下拉只列出启用项。被文章引用的标签不能删除（否则文章会丢标签），请改用停用；
          删除只对没被任何文章引用的标签开放。
        </p>
      </div>
    </div>

    <TagManager />
  </NSpace>
</template>
