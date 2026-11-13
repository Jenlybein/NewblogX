<script setup lang="ts">
import { computed, ref } from "vue";

import { useGlassShardTitle } from "~/composables/useGlassShardTitle";
import { GLASS_SHARDS } from "~/utils/glassShards";

/**
 * 「碎玻璃大标题」——把站名渲染成九块裁切碎片叠印的整字。
 *
 * 为什么要有它：首屏大标题是全站唯一的编辑部式例外区（见 docs/adr 的首屏封面 ADR），
 * 这里把它的**呈现方式**换成了 lanrhyme.top 那种碎玻璃效果（还原对象与几何数据见
 * `utils/glassShards.ts`）。字标字体是首屏唯一的衬线例外 `--font-wordmark`
 * （自托管 Playfair Display 700 italic），其余标题仍是 `--font-title`（Sora）。
 *
 * 结构（自下而上）：
 *   1. `.sr-only` 真文本 —— 无障碍与 SEO 的唯一来源；
 *   2. 底字：白 10%→5% 的渐变裁字，只在碎片缝隙里露出来；
 *   3. 九块碎片：每块里放一份**完整**文字，靠 `clip-path` 只露自己那块；
 *      浮层碎片（z=2）叠 `backdrop-filter` 毛玻璃与 1px 边缘光；
 *   4. 点击时的白色柔光（mix-blend-screen）。
 *
 * 动效全部在 `useGlassShardTitle` 里：我在这里只渲染骨架，不放任何 rAF。
 * SSR 会把这些层都渲染出来，所以首屏 h1 在 HTML 里不是空的。
 */
const props = withDefaults(
  defineProps<{
    /** 标题文字（通常传品牌名大写）。 */
    text: string;
    /** 碎片数量，默认全长（9 块）；窄屏或长文案可以减档。 */
    maxShards?: number;
    /** 点击是否合成碎裂音，默认关（博客首屏不该突然出声）。 */
    sound?: boolean;
  }>(),
  { maxShards: GLASS_SHARDS.length, sound: false },
);

const root = ref<HTMLElement | null>(null);

const shards = computed(() => GLASS_SHARDS.slice(0, Math.max(1, Math.min(props.maxShards, GLASS_SHARDS.length))));

useGlassShardTitle(root, {
  text: () => props.text,
  sound: props.sound,
});
</script>

<template>
  <span ref="root" class="glass-title">
    <span class="sr-only">{{ text }}</span>
    <span class="glass-title__base" aria-hidden="true" data-glass-text>{{ text }}</span>
    <span class="glass-title__halo" aria-hidden="true" />
    <span class="glass-title__shards" aria-hidden="true">
      <span
        v-for="shard in shards"
        :key="shard.id"
        class="glass-title__shard"
        :class="{ 'glass-title__shard--overlay': shard.z > 1 }"
        :style="{ clipPath: shard.path, zIndex: shard.z }"
        data-glass-shard
      >
        <span class="glass-title__shard-inner">
          <span class="glass-title__shard-text" data-glass-text>{{ text }}</span>
        </span>
        <span class="glass-title__shard-edge" aria-hidden="true" :style="{ clipPath: shard.path }" />
      </span>
    </span>
  </span>
</template>

<style scoped>
/*
 * 颜色一律从首屏的反白语义 token 派生（`--color-on-inverse`），不写白色字面量：
 * 明暗两套主题下碎片都会跟着切，也不会踩 dark-surfaces 门禁的"硬编码白"。
 */
.glass-title {
  position: relative;
  display: inline-block;
  max-width: 100%;
  /* 字号仍继承 h1 的 clamp(2rem, 9vw, 6rem)；字体换成首屏字标专用的衬线族
     （--font-wordmark 是全站唯一的衬线例外，登记见 fonts.test.ts）。 */
  font-family: var(--font-wordmark);
  font-size: inherit;
  font-weight: 700;
  font-style: italic;
  /* 衬线的字形盒比行盒高：Playfair 的 ascent + descent = 1.082 + 0.251 = 1.333em。
     碎片的多边形只覆盖这个盒子（0%–100%），行高小于字形盒时会把字的上缘和下伸部
     直接裁掉（J / y 的尾巴）。所以行高取字形盒高度，让裁切盒正好包住字形。 */
  line-height: 1.34;
  letter-spacing: -0.015em;
  text-transform: none;
  cursor: crosshair;
  perspective: 1000px;
  user-select: none;
  -webkit-user-select: none;
}

/* 底字：极淡的渐变裁字，只在碎片缝里露出。 */
.glass-title__base {
  position: relative;
  z-index: 0;
  display: block;
  padding-right: 0.15em;
  white-space: nowrap;
  color: transparent;
  -webkit-text-fill-color: transparent;
  background-image: linear-gradient(
    90deg,
    color-mix(in srgb, var(--color-on-inverse) 10%, transparent),
    color-mix(in srgb, var(--color-on-inverse) 5%, transparent)
  );
  -webkit-background-clip: text;
  background-clip: text;
}

/* 点击时在字后扩散的白色柔光（mix-blend-screen + 模糊）。
   过渡只动 scale / opacity（合成层属性）：宽高变化会逼着每帧重排重绘。 */
.glass-title__halo {
  position: absolute;
  top: 50%;
  left: 50%;
  z-index: 0;
  width: 120%;
  height: 150%;
  translate: -50% -50%;
  scale: 0.42;
  pointer-events: none;
  opacity: 0;
  mix-blend-mode: screen;
  background: radial-gradient(
    ellipse,
    color-mix(in srgb, var(--color-on-inverse) 25%, transparent) 0%,
    color-mix(in srgb, var(--color-on-inverse) 5%, transparent) 40%,
    transparent 70%
  );
  filter: blur(20px);
  transition:
    scale var(--motion-slow) ease-out,
    opacity var(--motion-slow) ease-out;
}

.glass-title__halo.is-active {
  scale: 1;
  opacity: 1;
  transition-duration: var(--motion-fast);
}

/* 碎片层：铺满文字盒，3D 上下文让 rotateX/Y 有透视。 */
.glass-title__shards {
  position: absolute;
  inset: 0;
  z-index: 10;
  width: 100%;
  height: 100%;
  perspective: 1000px;
}

/* 单块碎片：clip-path 在模板里按 GLASS_SHARDS 内联，这里只管 3D 上下文。 */
.glass-title__shard {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  transform-style: preserve-3d;
  will-change: transform;
}

/* 浮层碎片：毛玻璃 + 淡反白底 + 1px 内边缘光（截图里那几片"斜的玻璃"）。 */
.glass-title__shard--overlay {
  background: color-mix(in srgb, var(--color-on-inverse) 3%, transparent);
  -webkit-backdrop-filter: blur(16px) brightness(1.15);
  backdrop-filter: blur(16px) brightness(1.15);
}

.glass-title:hover .glass-title__shard--overlay {
  background: color-mix(in srgb, var(--color-on-inverse) 5%, transparent);
}

/* 碎片里的字：与底字同一套度量，所以九份拼起来严丝合缝。 */
.glass-title__shard-inner {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

.glass-title__shard-text {
  display: block;
  width: 100%;
  padding-right: 0.15em;
  white-space: nowrap;
  color: color-mix(in srgb, var(--color-on-inverse) 88%, transparent);
  text-shadow: 0 2px 10px color-mix(in srgb, var(--color-on-inverse) 20%, transparent);
}

.glass-title__shard-edge {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

/* 边缘光只给浮层：九块都加的话，inset 阴影的并集正好铺满整个盒子，
   会在字外糊出一圈淡矩形（原站也只给 .is-overlay 加）。 */
.glass-title__shard--overlay .glass-title__shard-edge {
  box-shadow:
    inset 0 0 0 1px color-mix(in srgb, var(--color-on-inverse) 8%, transparent),
    inset 0 0 20px color-mix(in srgb, var(--color-on-inverse) 2%, transparent);
}

@media (prefers-reduced-motion: reduce) {
  .glass-title__halo {
    transition: none;
  }
}
</style>
