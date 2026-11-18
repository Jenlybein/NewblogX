<script setup lang="ts">
import { computed, ref, watch } from "vue";

/**
 * 封面只有一个形态：有图就渲染图（16:9 居中裁切，票 204）；**没图就没有节点**（票 440）。
 *
 * 缺封面 / 加载失败曾经画过两种兜底（同底色空白、首字墨片），都被否掉了：
 * 空白的 16:9 框读起来像"图挂了"，首字墨片又显突兀。现在的口径是"没有就不显示"，
 * 整个封面槽由消费方按 `useCoverVisibility` 拿掉，换成无封面排版。
 */
const emit = defineEmits<{ fail: [] }>();

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

function handleError(): void {
  imageFailed.value = true;
  // 消费方据此把整个封面槽换掉（没封面就不该留框，见 `useCoverVisibility`）。
  emit("fail");
}
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
    @error="handleError"
  />
</template>

<style scoped>
/*
 * 封面的形状只有一个：16:9、高度由比例决定。上传时已经在后台/创作页定形成这个比例，
 * 读者侧只做固定居中的 cover，不再消费任何取景参数；未定形过的旧封面也按居中显示。
 */
.article-cover-image {
  display: block;
  width: 100%;
  aspect-ratio: 16 / 9;
  object-fit: cover;
  object-position: center;
}
</style>
