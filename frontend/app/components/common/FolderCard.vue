<script setup lang="ts">
import { useId } from "vue";

import ArticleCover from "~/components/article/ArticleCover.vue";
import {
  FOLDER_FLAP_HEIGHT,
  FOLDER_FLAP_PATH,
  FOLDER_FLAP_WIDTH,
  folderCardStyleVars,
} from "~/utils/folderCardGeometry";

/**
 * 合集/项目文件夹卡（术语见 GLOSSARY「合集/项目文件夹卡」，口径见阅读页 UI 记录
 * §三 3.6，票 293）。
 *
 * 形态（自下而上四层，缺一层就不像文件夹）：
 *   1. 背板：圆角实体面 + 内阴影，顶边比前盖低一点、底边露出 6px 后沿；
 *   2. 封面卡：**一张** 16:9 封面，夹在背板与前盖之间，常态露出上 2/3；
 *   3. 前盖玻璃：参考组件那条**带凸舌的轮廓**（左上圆角 / 45° 斜切肩 / 右侧平顶 /
 *      底部圆角），遮罩裁形 + backdrop-filter，铰链在下沿，常态前倾 15°；
 *   4. 前盖的墨线：轮廓描边 + SVG 内阴影（feOffset + feComposite + feColorMatrix）。
 *
 * 这里刻意**只有一张**封面——三张扇形方案已被否（grill 记录 §三 3.6）。
 * 几何全部来自 `~/utils/folderCardGeometry`：每一处位置都是相对容器的百分比，
 * 所以列宽变了形状不变，没有写死的 321×270 像素。
 *
 * 间距/位移只动 transform；前盖是内容面上**唯一**允许的玻璃层
 * （ADR-0011 后续修订 2026-11-07），因此必须自带两条不透明降级与窄屏 12px 模糊。
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

/**
 * 内阴影过滤器的 id：同一个页面上会同时有多张卡，`<filter>` 必须各自唯一，
 * 否则浏览器只认第一个（换主题或换列宽后阴影会跟错元素）。
 */
const shadeId = `folder-flap-shade-${useId().replace(/[^\w-]/gu, "-")}`;
</script>

<template>
  <li class="folder-card">
    <NuxtLink :to="href" class="folder-card__link">
      <span class="folder-card__stage" :style="geometry">
        <!-- ① 背板：文件夹的后片，封面落在它上面，前盖的玻璃也透它。 -->
        <span class="folder-card__back" aria-hidden="true" />

        <!-- ② 封面：只有一张，16:9 由 ArticleCover 撑开。 -->
        <span class="folder-card__sheet">
          <ArticleCover :src="cover" :title="title" />
        </span>

        <!-- ③④ 前盖：玻璃（遮罩裁成凸舌轮廓）+ 墨线（描边与内阴影）。 -->
        <span class="folder-card__flap" aria-hidden="true">
          <span class="folder-card__flap-glass" />
          <svg
            class="folder-card__flap-ink"
            :viewBox="`0 0 ${FOLDER_FLAP_WIDTH} ${FOLDER_FLAP_HEIGHT}`"
            preserveAspectRatio="none"
            aria-hidden="true"
          >
            <defs>
              <!--
                内阴影（参考组件的形状，不搬它的配色）：把自身 alpha 偏一点，
                取「自身 − 偏移」的边带 → 模糊 → 裁回形状内，再用全是数字的
                feColorMatrix 上色。矩阵里没有颜色字面量，换主题不用改这里。
              -->
              <filter
                :id="shadeId"
                x="-5%"
                y="-5%"
                width="110%"
                height="110%"
                color-interpolation-filters="sRGB"
              >
                <!-- 上沿高光 -->
                <feOffset in="SourceAlpha" dx="0" dy="10" result="lightOffset" />
                <feComposite
                  in="SourceAlpha"
                  in2="lightOffset"
                  operator="out"
                  result="lightEdge"
                />
                <feGaussianBlur in="lightEdge" stdDeviation="9" result="lightBlur" />
                <feComposite in="lightBlur" in2="SourceAlpha" operator="in" result="lightClip" />
                <feColorMatrix
                  in="lightClip"
                  type="matrix"
                  values="0 0 0 0 1 0 0 0 0 1 0 0 0 0 1 0 0 0 0.55 0"
                  result="light"
                />
                <!-- 下沿内阴影 -->
                <feOffset in="SourceAlpha" dx="0" dy="-6" result="shadeOffset" />
                <feComposite
                  in="SourceAlpha"
                  in2="shadeOffset"
                  operator="out"
                  result="shadeEdge"
                />
                <feGaussianBlur in="shadeEdge" stdDeviation="7" result="shadeBlur" />
                <feComposite in="shadeBlur" in2="SourceAlpha" operator="in" result="shadeClip" />
                <feColorMatrix
                  in="shadeClip"
                  type="matrix"
                  values="0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0.16 0"
                  result="shade"
                />
                <feMerge>
                  <feMergeNode in="shade" />
                  <feMergeNode in="light" />
                </feMerge>
              </filter>
            </defs>
            <path class="folder-card__flap-edge" :d="FOLDER_FLAP_PATH" />
            <path
              class="folder-card__flap-shade"
              :d="FOLDER_FLAP_PATH"
              :filter="`url(#${shadeId})`"
            />
          </svg>
        </span>
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

/*
 * 背板：文件夹的后片。比前盖低一点（露一条后沿），底部也多留 6px，
 * 前盖盖上后就是"两层纸"；内阴影让后片凹进去。
 */
.folder-card__back {
  position: absolute;
  top: var(--folder-back-top);
  left: 0;
  z-index: 0;
  width: 100%;
  height: var(--folder-back-height);
  border: 1px solid var(--color-line);
  border-radius: var(--radius-xl);
  background: var(--color-bg-subtle);
  box-shadow:
    inset 0 4px 10px -3px var(--color-shadow),
    inset 0 -1px 0 var(--color-line-faint),
    0 20px 34px -22px var(--color-shadow);
}

/*
 * 封面卡：插在背板与前盖之间。底色取抬升面（没有封面时就是一张空白纸，
 * 不渲染破图、不留文字），图片由 ArticleCover 撑成 16:9。
 */
.folder-card__sheet {
  position: absolute;
  top: var(--folder-sheet-top);
  left: var(--folder-sheet-left);
  z-index: 1;
  width: var(--folder-sheet-width);
  height: var(--folder-sheet-height);
  overflow: hidden;
  border: 1px solid var(--color-line-faint);
  border-radius: var(--radius-sm);
  background: var(--color-surface-raised);
  box-shadow: 0 10px 20px -14px var(--color-shadow);
  transform: translateY(0) rotate(0deg);
  transition: transform var(--motion-base) var(--motion-ease);
}

/*
 * 前盖：铰链在下沿、往读者方向前倾（rotateX 负角 = 顶边朝观者压下来，
 * 于是斜切肩把封面放出来）。它就是文件夹的轮廓，所以 transform 打在容器上，
 * 玻璃与墨线两层跟着一起动。
 */
.folder-card__flap {
  position: absolute;
  top: var(--folder-flap-top);
  left: 0;
  z-index: 2;
  width: 100%;
  height: var(--folder-flap-height);
  transform-origin: 50% 100%;
  transform: rotateX(var(--folder-flap-tilt));
  transition: transform var(--motion-base) var(--motion-ease);
}

/*
 * 前盖投在背板与封面上的影子：同一条轮廓当遮罩，填阴影色再糊开。
 * 没有它，前盖就是"描了个边"，前后两层读不出厚度——这一个影子占"像文件夹"的一半。
 * 两层叠着来：近的一层压出硬边，远的一层摊开成环境影。
 */
.folder-card__flap::before,
.folder-card__flap::after {
  position: absolute;
  inset: 0;
  z-index: 0;
  background: var(--color-shadow);
  content: "";
  mask-image: var(--folder-flap-mask);
  mask-repeat: no-repeat;
  mask-size: 100% 100%;
  -webkit-mask-image: var(--folder-flap-mask);
  -webkit-mask-repeat: no-repeat;
  -webkit-mask-size: 100% 100%;
}

.folder-card__flap::before {
  filter: blur(7px);
  transform: translateY(6px);
}

.folder-card__flap::after {
  filter: blur(16px);
  transform: translateY(14px);
}

/*
 * 玻璃层：内容面上唯一的玻璃（登记例外）。用参考轮廓当遮罩裁形——
 * 直角背板之所以不像文件夹，差的就是这一步。填充/模糊全走玻璃 token。
 */
.folder-card__flap-glass {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: block;
  background: var(--glass-thick-fill);
  backdrop-filter: saturate(var(--glass-thick-saturate)) blur(var(--glass-thick-blur));
  -webkit-backdrop-filter: saturate(var(--glass-thick-saturate)) blur(var(--glass-thick-blur));
  mask-image: var(--folder-flap-mask);
  mask-size: 100% 100%;
  mask-repeat: no-repeat;
  -webkit-mask-image: var(--folder-flap-mask);
  -webkit-mask-size: 100% 100%;
  -webkit-mask-repeat: no-repeat;
}

/* 墨线层：轮廓描边 + SVG 内阴影，跟着前盖一起画（不受遮罩影响）。 */
.folder-card__flap-ink {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: block;
  width: 100%;
  height: 100%;
  overflow: visible;
}

.folder-card__flap-edge {
  fill: none;
  stroke: var(--color-line-strong);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}

.folder-card__flap-shade {
  fill: currentColor;
  color: var(--color-line-faint);
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
  gap: 0.375rem;
  padding: 0 0.125rem;
}

/* 超长标题最多两行、描述固定两行：卡片高度由网格撑平，不被文本撑破。
   2026-11-12 起列宽从 310px 收到约 200px，字号整体跟降一档，
   否则标题会在六列里换行成三行、把整行卡片撑高。 */
.folder-card__title {
  display: -webkit-box;
  overflow: hidden;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1.0625rem;
  font-weight: 600;
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
  font-size: 0.8125rem;
  line-height: 1.55;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.folder-card__meta {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.08em;
}

/* 降级 ①：不支持 backdrop-filter 的浏览器 → 前盖退化为不透明面。 */
@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
  .folder-card__flap-glass {
    background: var(--color-surface-raised);
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }
}

/* 降级 ②：系统「减少透明度」→ 同上（玻璃变成实心面，几何与交互都不变）。 */
@media (prefers-reduced-transparency: reduce) {
  .folder-card__flap-glass {
    background: var(--color-surface-raised);
    backdrop-filter: none;
    -webkit-backdrop-filter: none;
  }
}

/* 降级 ③：窄屏把模糊压到 12px（ADR-0011 约束 7 的移动端上限）。 */
@media (max-width: 768px) {
  .folder-card__flap-glass {
    backdrop-filter: saturate(var(--glass-thick-saturate)) blur(var(--glass-blur-mobile));
    -webkit-backdrop-filter: saturate(var(--glass-thick-saturate)) blur(var(--glass-blur-mobile));
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
