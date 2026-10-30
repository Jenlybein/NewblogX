<script setup lang="ts">
import { computed } from "vue";
import { IconMoonStars, IconSunHigh } from "@tabler/icons-vue";
import { NButton, NIcon } from "naive-ui";

/**
 * 后台的明暗开关（顶栏 / 登录页 / 初始化页共用）。
 *
 * 为什么必须在每个页面都能看到（ADR-0012 约束 11）：后台的信息密度最高，
 * 暗色下如果某个页面（尤其登录/初始化这种"进站第一眼"）没有开关，用户就只能
 * 回仪表盘才能切回去 —— 那是可用性问题，不是审美问题。
 *
 * 图标表示"当前处于哪种主题"，aria-label 说清"点了会切到哪种"
 * （只写"切换主题"对读屏软件没有信息量）。
 */
const themeStore = useThemeStore();

const isDark = computed(() => themeStore.theme === "dark");
const themeLabel = computed(() => (isDark.value ? "切换到浅色主题" : "切换到暗色主题"));
</script>

<template>
  <NButton
    class="admin-theme-toggle"
    secondary
    circle
    size="small"
    :aria-label="themeLabel"
    :title="themeLabel"
    @click="themeStore.toggleTheme()"
  >
    <template #icon>
      <NIcon :size="18">
        <!-- 暗色下显示太阳（提示"可以切回浅色"），浅色下显示月亮。 -->
        <IconSunHigh v-if="isDark" />
        <IconMoonStars v-else />
      </NIcon>
    </template>
  </NButton>
</template>
