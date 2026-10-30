<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { BRAND } from "~/utils/brand";

const props = withDefaults(
  defineProps<{
    src?: string | null;
    title?: string | null;
    label?: string;
  }>(),
  {
    src: "",
    title: "文章封面",
    label: BRAND.name.en,
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
/*
 * 封面兜底：中性石墨渐变（深墨面 → 极淡石墨）+ 反白字，明暗两态都由语义层承载。
 * 之前是克莱因蓝 135deg 渐变；#20 起不再出现任何原始色相。
 */
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
  border-radius: var(--radius-md);
  background:
    radial-gradient(circle at 20% 18%, var(--color-on-inverse-muted), transparent 22%),
    linear-gradient(135deg, var(--color-surface-inverse) 0%, var(--color-surface-inverse) 42%, var(--color-ink-faint) 100%);
  color: var(--color-on-inverse);
}

.article-cover-default__grid {
  position: absolute;
  inset: 0;
  opacity: 0.18;
  background-image:
    linear-gradient(var(--color-on-inverse) 1px, transparent 1px),
    linear-gradient(90deg, var(--color-on-inverse) 1px, transparent 1px);
  background-size: 22px 22px;
  mask-image: linear-gradient(135deg, var(--color-ink), transparent 72%);
}

.article-cover-default__orb {
  position: absolute;
  border-radius: var(--radius-pill);
  filter: blur(2px);
}

.article-cover-default__orb--one {
  right: -28px;
  top: -20px;
  width: 96px;
  height: 96px;
  background: var(--color-on-inverse-muted);
}

.article-cover-default__orb--two {
  left: -34px;
  bottom: -38px;
  width: 120px;
  height: 120px;
  background: var(--color-ink-faint);
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
  border: 1px solid var(--color-on-inverse-muted);
  border-radius: var(--radius-pill);
  padding: 3px 8px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  background: var(--color-surface-inverse);
}

.article-cover-default__content strong {
  font-size: 18px;
  line-height: 1;
  letter-spacing: -0.04em;
  text-shadow: 0 8px 24px var(--color-shadow);
}
</style>
