<script setup lang="ts">
import { computed, ref } from "vue";
import { IconChevronLeft, IconChevronRight } from "@tabler/icons-vue";
import { NButton } from "naive-ui";
import type { BannerItem } from "~/types/api";

const props = defineProps<{
  banners: BannerItem[];
}>();

const currentIndex = ref(0);

const currentBanner = computed(() => props.banners[currentIndex.value] ?? null);

function switchBanner(step: number) {
  if (!props.banners.length) return;
  currentIndex.value = (currentIndex.value + step + props.banners.length) % props.banners.length;
}
</script>

<template>
  <div class="banner-shell">
    <div class="banner-stage">
      <img
        v-if="currentBanner"
        :src="currentBanner.cover"
        :alt="`轮播图 ${currentIndex + 1}`"
      />
      <div v-else class="flex h-full min-h-[320px] items-center justify-center bg-slate-100 text-slate-400 dark:bg-slate-900 dark:text-slate-500">
        暂无轮播内容
      </div>

      <div class="banner-nav is-prev" v-if="props.banners.length > 1">
        <NButton circle secondary @click="switchBanner(-1)">
          <template #icon>
            <IconChevronLeft :size="18" />
          </template>
        </NButton>
      </div>
      <div class="banner-nav is-next" v-if="props.banners.length > 1">
        <NButton circle secondary @click="switchBanner(1)">
          <template #icon>
            <IconChevronRight :size="18" />
          </template>
        </NButton>
      </div>

      <div class="banner-dots" v-if="props.banners.length > 1">
        <button
          v-for="(_, index) in props.banners"
          :key="index"
          type="button"
          class="banner-dot h-2 rounded-full transition-all"
          :class="index === currentIndex ? 'is-active w-7' : 'w-2'"
          @click="currentIndex = index"
        />
      </div>
    </div>
  </div>
</template>
