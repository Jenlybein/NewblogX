<script setup lang="ts">
/**
 * 评论管理（`/articles/comments`）：管理端处置读者站评论（2026-11-27）。
 *
 * 挂在「文章管理」下：评论是文章下面的内容，运营找"某篇文章下的垃圾评论"
 * 时不会先去想它属于哪个独立分组。
 */
import CommentManager from "~/components/articles/CommentManager.vue";
</script>

<template>
  <CommentManager />
</template>
