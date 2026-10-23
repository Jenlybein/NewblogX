<script setup lang="ts">
import { computed, ref, watch } from "vue";

const props = withDefaults(
  defineProps<{
    src?: string | null;
    title?: string | null;
    label?: string;
  }>(),
  {
    src: "",
    title: "文章封面",
    label: "BlogX",
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
  <div v-else class="article-cover-default" role="img" :aria-label="`${title || '文章'}暂无封面`">
    <div class="article-cover-default__grid" />
    <div class="article-cover-default__orb article-cover-default__orb--one" />
    <div class="article-cover-default__orb article-cover-default__orb--two" />
    <div class="article-cover-default__content">
      <span>{{ label }}</span>
      <strong>NO COVER</strong>
    </div>
  </div>
</template>

<style scoped>
.article-cover-image,
.article-cover-default {
  display: block;
  width: 100%;
  height: 100%;
}

.article-cover-image {
  object-fit: cover;
  object-position: center;
}

.article-cover-default {
  position: relative;
  overflow: hidden;
  background:
    radial-gradient(circle at 20% 18%, rgba(255, 255, 255, 0.78), transparent 22%),
    linear-gradient(135deg, #123f3c 0%, #2d746c 42%, #f4e9d6 100%);
  color: rgba(255, 255, 255, 0.92);
}

.article-cover-default__grid {
  position: absolute;
  inset: 0;
  opacity: 0.18;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.5) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.5) 1px, transparent 1px);
  background-size: 22px 22px;
  mask-image: linear-gradient(135deg, black, transparent 72%);
}

.article-cover-default__orb {
  position: absolute;
  border-radius: 999px;
  filter: blur(2px);
}

.article-cover-default__orb--one {
  right: -28px;
  top: -20px;
  width: 96px;
  height: 96px;
  background: rgba(255, 255, 255, 0.3);
}

.article-cover-default__orb--two {
  left: -34px;
  bottom: -38px;
  width: 120px;
  height: 120px;
  background: rgba(217, 119, 6, 0.28);
}

.article-cover-default__content {
  position: absolute;
  inset: auto 14px 14px 14px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.article-cover-default__content span {
  width: fit-content;
  border: 1px solid rgba(255, 255, 255, 0.38);
  border-radius: 999px;
  padding: 3px 8px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  background: rgba(255, 255, 255, 0.14);
  backdrop-filter: blur(8px);
}

.article-cover-default__content strong {
  font-size: 18px;
  line-height: 1;
  letter-spacing: -0.04em;
  text-shadow: 0 8px 24px rgba(15, 23, 42, 0.2);
}
</style>
