<script setup lang="ts">
import { computed } from "vue";

import { brandCopyright } from "~/utils/brand";

const copyrightLine = computed(() => brandCopyright(new Date().getFullYear()));
</script>

<template>
  <footer class="page-shell pt-0">
    <div class="flex flex-col gap-3 border-t border-slate-200/70 px-2 py-6 text-sm muted dark:border-slate-800/80 md:flex-row md:items-center md:justify-between">
      <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
        <span>{{ copyrightLine }}</span>
        <span>粤ICP备 2026000000 号</span>
      </div>
      <nav class="flex flex-wrap items-center gap-x-4 gap-y-2">
        <NuxtLink to="/">首页</NuxtLink>
        <NuxtLink to="/search">搜索</NuxtLink>
      </nav>
    </div>
  </footer>
</template>
