<script setup lang="ts">
import ArticleCover from "~/components/article/ArticleCover.vue";
import { folderCardStyleVars } from "~/utils/folderCardGeometry";

/**
 * 合集/项目文件夹卡（术语见 GLOSSARY「合集/项目文件夹卡」，口径见
 * docs/notes/2026-11-07-reading-ui-grill.md §三 3.6，票 293）。
 *
 * 形态：背板 + **一张** 16:9 封面卡 + 前盖（flap）。封面常态露出上 2/3，
 * 前盖从下盖住下沿；hover / 键盘聚焦时封面上抽并旋转、前盖前倾；点击直接进详情。
 * 这里刻意**只有一张**封面——三张扇形方案已被否（grill 记录 §三 3.6）。
 *
 * 几何全部来自 `~/utils/folderCardGeometry`：卡内每一处位置都是相对容器的百分比，
 * 所以列宽变了形状不变，没有写死的 321×270 像素。
 *
 * 间距/位移只动 transform；前盖是内容面上**唯一**允许的玻璃层
 * （ADR-0012 后续修订 2026-11-07），因此必须自带两条不透明降级与窄屏 12px 模糊。
 * 本票不参与「弹簧」三处例外：位移走 200ms CSS 过渡 + 唯一缓动 token。
 */
interface FolderCardProps {
  /** 详情页路由，整张卡就是这一条链接。 */
  href: string;
  title?: string;
  description?: string;
  /** 封面 URL；空串走 ArticleCover 的空占位（不渲染破图）。 */
  cover?: string;
  /** 篇数文案（"N 篇文章 / 暂无文章"）。 */
  meta?: string;
}

withDefaults(defineProps<FolderCardProps>(), {
  title: "",
  description: "",
  cover: "",
  meta: "",
});

/** 几何是常量：每张卡拿到的比例一样，不随实例或视口变。 */
const geometry = folderCardStyleVars();
</script>

<template>
  <li class="folder-card">
    <NuxtLink :to="href" class="folder-card__link">
      <span class="folder-card__stage" :style="geometry">
        <span class="folder-card__back" aria-hidden="true" />
        <span class="folder-card__sheet">
          <ArticleCover :src="cover" :title="title" />
        </span>
        <span class="folder-card__flap" aria-hidden="true" />
      </span>
      <span class="folder-card__body">
        <strong class="folder-card__title">{{ title }}</strong>
        <span class="folder-card__description">{{ description }}</span>
        <span class="folder-card__meta">{{ meta }}</span>
      </span>
    </NuxtLink>
  </li>
</template>

<style scoped>
/*
 * 文字区沿用既有卡片的内容面样式（语义色 token + 圆角阶梯）；文件夹本身
 * 是背板 / 封面 / 前盖三层，不套外框——再包一层边框就成了"框里的文件夹"。
 */
.folder-card {
  display: block;
  min-width: 0;
  list-style: none;
}

.folder-card__link {
  display: flex;
  flex-direction: column;
  gap: 0.875rem;
  height: 100%;
  color: inherit;
  text-decoration: none;
}

.folder-card__link:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 3px;
  border-radius: var(--radius-lg);
}

/* 舞台：比例由几何助手给出；透视让前盖的 rotateX 读成"前倾"而不是被压扁。 */
.folder-card__stage {
  position: relative;
  display: block;
  width: 100%;
  aspect-ratio: var(--folder-stage-ratio);
  perspective: 900px;
}

/* 背板：不透明面。空封面就落在这块底色上（封面卡自身没有底色）。 */
.folder-card__back {
  position: absolute;
  left: var(--folder-back-left);
  top: var(--folder-back-top);
  z-index: 0;
  width: var(--folder-back-width);
  height: var(--folder-back-height);
  border: 1px solid var(--color-line);
  border-radius: var(--radius-lg);
  background: var(--color-bg-subtle);
  box-shadow: 0 20px 44px -34px var(--color-shadow);
}

/*
 * 封面卡：插在背板与前盖之间。透明底 + 圆角裁切，图片由 ArticleCover 撑成 16:9；
 * 没有封面时只剩一块透出的背板底色（无破图、无文字）。
 */
.folder-card__sheet {
  position: absolute;
  left: var(--folder-sheet-left);
  top: var(--folder-sheet-top);
  z-index: 1;
  width: var(--folder-sheet-width);
  height: var(--folder-sheet-height);
  border-radius: var(--radius-md);
  overflow: hidden;
  transform: translateY(0) rotate(0deg);
  transition: transform var(--motion-base) var(--motion-ease);
}

/* 前盖：内容面上唯一的玻璃层（登记例外）。铰链在下沿，往读者方向前倾。 */
.folder-card__flap {
  position: absolute;
  left: var(--folder-flap-left);
  top: var(--folder-flap-top);
  z-index: 2;
  width: var(--folder-flap-width);
  height: var(--folder-flap-height);
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md) var(--radius-md) var(--radius-lg) var(--radius-lg);
  background: var(--glass-thin-fill);
  backdrop-filter: saturate(var(--glass-thin-saturate)) blur(var(--glass-thin-blur));
  -webkit-backdrop-filter: saturate(var(--glass-thin-saturate)) blur(var(--glass-thin-blur));
  box-shadow:
    inset 0 1px 0 var(--glass-rim),
    0 18px 34px -28px var(--color-shadow);
  transform-origin: 50% 100%;
  transform: rotateX(var(--folder-flap-tilt));
  transition: transform var(--motion-base) var(--motion-ease);
}

.folder-card__link:hover .folder-card__sheet,
.folder-card__link:focus-visible .folder-card__sheet {
  transform: translateY(var(--folder-sheet-lift)) rotate(var(--folder-sheet-tilt));
}

.folder-card__link:hover .folder-card__flap,
.folder-card__link:focus-visible .folder-card__flap {
  transform: rotateX(var(--folder-flap-tilt-open));
}

.folder-card__body {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0 0.25rem;
}

/* 超长标题最多两行、描述固定两行：卡片高度由网格撑平，不被文本撑破。 */
.folder-card__title {
  display: -webkit-box;
  overflow: hidden;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1.25rem;
  letter-spacing: -0.015em;
  line-height: 1.3;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.folder-card__description {
  display: -webkit-box;
  flex: 1 1 auto;
  overflow: hidden;
  color: var(--color-ink-muted);
  font-size: 0.875rem;
  line-height: 1.6;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.folder-card__meta {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.08em;
}

/* 降级 ①：不支持 backdrop-filter 的浏览器 → 前盖退化为不透明面。 */
@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
  .folder-card__flap {
    background: var(--color-surface-raised);
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }
}

/* 降级 ②：系统「减少透明度」→ 同上（玻璃变成实心面，几何与交互都不变）。 */
@media (prefers-reduced-transparency: reduce) {
  .folder-card__flap {
    background: var(--color-surface-raised);
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }
}

/* 降级 ③：窄屏把模糊压到 12px（ADR-0012 约束 7 的移动端上限）。 */
@media (max-width: 768px) {
  .folder-card__flap {
    backdrop-filter: saturate(var(--glass-thin-saturate)) blur(var(--glass-blur-mobile));
    -webkit-backdrop-filter: saturate(var(--glass-thin-saturate)) blur(var(--glass-blur-mobile));
  }
}

/* 降级 ④：减少动态 → 无位移与旋转，只保留静态形态（归零，不是放慢）。 */
@media (prefers-reduced-motion: reduce) {
  .folder-card__sheet,
  .folder-card__flap {
    transition: none;
  }

  .folder-card__link:hover .folder-card__sheet,
  .folder-card__link:focus-visible .folder-card__sheet {
    transform: none;
  }

  .folder-card__link:hover .folder-card__flap,
  .folder-card__link:focus-visible .folder-card__flap {
    transform: rotateX(var(--folder-flap-tilt));
  }
}
</style>
