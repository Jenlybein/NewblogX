<script setup lang="ts">
import { computed } from "vue";

import BrandLogo from "~/components/brand/Logo.vue";
import BrandWordmark from "~/components/brand/Wordmark.vue";
import { useMotionPreference } from "~/composables/useMotionPreference";
import { BRAND, brandCopyright } from "~/utils/brand";
import { FOOTER_ICP, footerNavItems } from "~/utils/chrome";

const links = footerNavItems();
const copyrightLine = computed(() => brandCopyright(new Date().getFullYear()));
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
        <!-- 扁平矢量果冻方块放在奶油白方板上：蓝铺色上仍然清晰，不用再造一套白色 logo -->
        <span class="site-footer__plate">
          <BrandLogo variant="flat" :size="34" decorative />
        </span>
        <div class="site-footer__lockup">
          <BrandWordmark size="lg" tone="invert" layout="stack" />
          <p class="site-footer__tagline">{{ BRAND.tagline }}</p>
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
        <span>{{ FOOTER_ICP }}</span>
      </div>
    </div>
  </footer>
</template>

<style scoped>
/*
 * 页脚 = 整块克莱因蓝铺色（形状语义「圆 = 克莱因蓝铺色块」），滚动到底的结束锚点。
 * 文字一律奶油白（主）/ 派生奶油白（次），对比度由组件测试用 WCAG 公式从
 * tokens.css 取真实色值计算（≥ 4.5:1）。
 */
.site-footer {
  margin-top: 3rem;
  background: var(--jelly-blue);
  color: var(--jelly-cream);
}

.site-footer__inner {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 1.25rem;
  padding-top: 2rem;
  padding-bottom: 1.75rem;
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
  background: var(--jelly-cream);
}

.site-footer__lockup {
  min-width: 0;
}

.site-footer__tagline {
  margin-top: 0.375rem;
  color: var(--jelly-cream-700);
  font-size: 0.8125rem;
  letter-spacing: 0.04em;
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
  color: var(--jelly-cream);
  font: inherit;
  font-size: 0.875rem;
  text-decoration: none;
  cursor: pointer;
  transition: border-color var(--motion-fast) var(--motion-ease);
}

.site-footer__link:hover,
.site-footer__link:focus-visible {
  border-bottom-color: var(--jelly-cream);
}

.site-footer__top {
  margin-left: auto;
}

.site-footer :is(a, button):focus-visible {
  outline: 2px solid var(--jelly-cream);
  outline-offset: 2px;
}

.site-footer__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.375rem 1.25rem;
  padding-top: 0.875rem;
  border-top: 1px solid var(--jelly-cream-700);
  color: var(--jelly-cream-700);
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
</style>
