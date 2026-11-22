<script setup lang="ts">
import { computed } from "vue";

import BrandLogo from "~/components/brand/Logo.vue";
import BrandWordmark from "~/components/brand/Wordmark.vue";
import { useMotionPreference } from "~/composables/useMotionPreference";
import { useSiteStore } from "~/stores/site";
import { brandCopyright } from "~/utils/brand";
import { footerNavItems } from "~/utils/chrome";
import { siteDateParts } from "~/utils/format";

const links = footerNavItems();
// 版权年份按站点时区取：页脚在 SSR 里渲染，跟本机时区走的话跨年夜会让服务端
// （UTC）与浏览器（UTC+8）各算一年，水合文本直接不一致（票 436）。
const copyrightLine = computed(() => brandCopyright(siteDateParts(new Date()).year));
const site = useSiteStore();
// 备案号来自运行时站点配置（后台可改），不再写死；空值 / 尚未取回时整行不渲染。
const beianLine = computed(() => (site.runtimeConfig?.site_info?.beian ?? "").trim());
const { prefersReducedMotion } = useMotionPreference();

function scrollToTop() {
  // 开启「减少动态」时不平滑滚动：直接跳回去（与 #3 的全局降级同一口径）。
  const behavior = prefersReducedMotion.value ? "auto" : "smooth";
  globalThis.scrollTo({ top: 0, behavior });
}
</script>

<template>
  <footer class="site-footer">
    <div class="page-shell site-footer__inner">
      <div class="site-footer__brand">
        <!-- 扁平矢量果冻方块放在反白方板上：深墨面铺色上仍然清晰 -->
        <span class="site-footer__plate">
          <BrandLogo variant="flat" :size="28" decorative />
        </span>
        <div class="site-footer__lockup">
          <BrandWordmark size="lg" tone="invert" layout="stack" />
        </div>
      </div>

      <nav class="site-footer__nav" aria-label="页脚导航">
        <NuxtLink v-for="item in links" :key="item.to" :to="item.to" class="site-footer__link">
          {{ item.label }}
        </NuxtLink>
        <button
          type="button"
          class="site-footer__link site-footer__top"
          data-chrome="to-top"
          @click="scrollToTop"
        >
          回到顶部
        </button>
      </nav>

      <div class="site-footer__meta">
        <span>{{ copyrightLine }}</span>
        <span v-if="beianLine">{{ beianLine }}</span>
      </div>
    </div>
  </footer>
</template>

<style scoped>
/*
 * 页脚 = 深墨面铺色（中性石墨阶的反白层），滚动到底的结束锚点。
 * 文字一律取反白语义 token（主反白 / 次级反白），对比度由组件测试用 WCAG 公式
 * 从 tokens.css 取真实色值计算（明暗两态 ≥ 4.5:1）。
 */
.site-footer {
  margin-top: 3rem;
  background: var(--color-surface-inverse);
  color: var(--color-on-inverse);
}

.site-footer__inner {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 1.25rem;
  padding-top: 2rem;
  /* 安全区（票 423 第 20 项）：iPhone 的 Home 指示条会压住页脚最后一行版权/备案号，
     所以底部内边距叠加 `env(safe-area-inset-bottom)`；不支持该变量的浏览器退化为
     原来的 1.75rem（不写死像素、也不会多抬）。 */
  padding-bottom: calc(1.75rem + env(safe-area-inset-bottom, 0px));
}

.site-footer__brand {
  display: flex;
  align-items: center;
  gap: 0.875rem;
  min-width: 0;
}

.site-footer__plate {
  display: inline-flex;
  flex: none;
  padding: 0.3125rem;
  border-radius: var(--radius-sm);
  background: var(--color-on-inverse);
}

.site-footer__lockup {
  min-width: 0;
}

.site-footer__nav {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.25rem 1.25rem;
}

.site-footer__link {
  display: inline-flex;
  align-items: center;
  min-height: 2.75rem;
  padding: 0;
  border: 0;
  border-bottom: 1px solid transparent;
  background: transparent;
  color: var(--color-on-inverse);
  font: inherit;
  font-size: 0.875rem;
  text-decoration: none;
  cursor: pointer;
  transition: border-color var(--motion-fast) var(--motion-ease);
}

.site-footer__link:hover,
.site-footer__link:focus-visible {
  border-bottom-color: var(--color-on-inverse);
}

.site-footer__top {
  margin-left: auto;
}

.site-footer :is(a, button):focus-visible {
  outline: 2px solid var(--color-on-inverse);
  outline-offset: 2px;
}

.site-footer__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.375rem 1.25rem;
  padding-top: 0.875rem;
  /* 版权行上方的 1px 线降到 14% 反白档（票 429）：此前借了次级字色的 72%，
   * 是页脚里最亮的一条线，比正文还抢眼。现在与全站发丝线同档，走语义 token
   * `--color-on-inverse-line`：它在 tokens.css 里由原始层反白三元组按 α 派生
   * （ADR-0011 约束 3；组件只消费语义层，不自己拼三元组）。也不再借 color-mix
   * 表达同一档，也不写裸 rgba。 */
  border-top: 1px solid var(--color-on-inverse-line);
  color: var(--color-on-inverse-muted);
  font-size: 0.8125rem;
  letter-spacing: 0.02em;
}

@media (min-width: 640px) {
  .site-footer__inner {
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
  }

  .site-footer__meta {
    grid-column: 1 / -1;
  }
}

/* 页脚收矮：移动端品牌与导航并排，底部只留一行备案。 */
.site-footer {
  margin-top: 2rem;
}

.site-footer__inner {
  gap: 0.75rem;
  padding-top: 1.25rem;
  padding-bottom: calc(1rem + env(safe-area-inset-bottom, 0px));
}

.site-footer__plate {
  padding: 0.25rem;
}

.site-footer__brand {
  gap: 0.5rem;
}

.site-footer__link {
  min-height: 2.75rem;
}

.site-footer__meta {
  padding-top: 0.625rem;
}

@media (max-width: 639px) {
  .site-footer__inner {
    grid-template-columns: auto minmax(0, 1fr);
    gap: 0.5rem 0.75rem;
    padding-top: 0.875rem;
    padding-bottom: calc(0.75rem + env(safe-area-inset-bottom, 0px));
  }

  .site-footer__nav {
    justify-self: end;
    gap: 0.125rem 0.75rem;
  }

  .site-footer__top {
    margin-left: 0;
  }

  .site-footer__meta {
    grid-column: 1 / -1;
    padding-top: 0.5rem;
  }
}
</style>

