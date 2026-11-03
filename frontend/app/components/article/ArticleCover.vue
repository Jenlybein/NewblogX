<script setup lang="ts">
import { computed, ref, watch } from "vue";

const props = withDefaults(
  defineProps<{
    src?: string | null;
    title?: string | null;
  }>(),
  {
    src: "",
    title: "文章封面",
  },
);

const imageFailed = ref(false);
const normalizedSrc = computed(() => props.src?.trim() || "");
const showImage = computed(() => Boolean(normalizedSrc.value) && !imageFailed.value);

watch(normalizedSrc, () => {
  imageFailed.value = false;
});
</script>

<template>
  <img
    v-if="showImage"
    class="article-cover-image"
    :src="normalizedSrc"
    :alt="title || '文章封面'"
    width="424"
    height="304"
    loading="lazy"
    @error="imageFailed = true"
  />
  <!-- 票 210：缺封面 / 加载失败都不是「一种内容」——只留一块与父容器同底色的 16:9 空白。 -->
  <div v-else class="article-cover-default" aria-hidden="true" />
</template>

<style scoped>
/*
 * 封面兜底（票 210）：缺封面 / 加载失败只留一块与父容器同底色的 16:9 空白——
 * 无文字、无图标、无纹理，且对辅助技术隐藏（aria-hidden 写在模板上）。
 * 底纹、光斑与品牌小签整套退场，占位不再消费任何颜色 token。
 *
 * 封面的形状只有一个：16:9、高度由比例决定（票 204）。上传时已经在后台/创作页
 * 定形成这个比例，读者侧只做固定居中的 cover，不再消费任何取景参数；
 * 未定形过的旧封面也按居中显示，不拉伸、不留白。
 */
.article-cover-image,
.article-cover-default {
  display: block;
  width: 100%;
  aspect-ratio: 16 / 9;
}

.article-cover-image {
  object-fit: cover;
  object-position: center;
}

.article-cover-default {
  background: transparent;
}
</style>
