<script setup lang="ts">
import { computed, defineAsyncComponent, ref, watch } from "vue";
import { NCard, NModal, NTabPane, NTabs } from "naive-ui";
import { useUiStore } from "~/stores/ui";
import { useSiteStore } from "~/stores/site";
import { usePointerSpecular } from "~/composables/usePointerSpecular";
import { BRAND } from "~/utils/brand";

type AuthTabName = "phone" | "email-login" | "password";

const PhoneAuthForm = defineAsyncComponent({
  loader: () => import("~/components/auth/PhoneAuthForm.vue"),
  suspensible: false,
});
const PasswordLoginForm = defineAsyncComponent({
  loader: () => import("~/components/auth/PasswordLoginForm.vue"),
  suspensible: false,
});
const EmailLoginForm = defineAsyncComponent({
  loader: () => import("~/components/auth/EmailLoginForm.vue"),
  suspensible: false,
});

const uiStore = useUiStore();
const siteStore = useSiteStore();
// 弹窗是厚玻璃浮层：指针镜面高光由统一 composable 写入 --mx/--my（移动端 / 减少动态关闭）。
const { handlePointerMove } = usePointerSpecular();

/**
 * 默认 tab 必须跟着站点开关走。
 *
 * 手机号是本站的注册入口，但它默认是关的（没配阿里云短信就打开只会让用户点出错）。
 * 把「手机号」写死成默认 tab 的后果是：站长没开这个开关时，访客一开弹窗就吃到
 * 「站点未启用手机号登录」——注册入口直接坏掉。
 *
 * 站点配置还没取回来时按「密码登录优先」兜底：它在本项目的默认配置里是开的，
 * 而且任何时刻都不会因为缺配置而报「未启用」。
 */
const phoneLoginEnabled = computed(() => siteStore.runtimeConfig?.login?.phone_login === true);
const passwordLoginEnabled = computed(() => siteStore.runtimeConfig?.login?.username_pwd_login !== false);
const emailLoginEnabled = computed(() => siteStore.runtimeConfig?.login?.email_login === true);

function preferredTab(): AuthTabName {
  if (phoneLoginEnabled.value) return "phone";
  if (passwordLoginEnabled.value) return "password";
  if (emailLoginEnabled.value) return "email-login";
  // 三种都关掉是配置错误；仍落在手机号上，让服务端给出明确错误而不是空白弹窗。
  return "phone";
}

const activeTab = ref<AuthTabName>(preferredTab());
const activatedTabs = ref<AuthTabName[]>([activeTab.value]);
// 用户手动切过 tab 之后，就不再用"配置到位"覆盖他的选择。
let userPickedTab = false;
const authTitle = computed(() => `加入 ${BRAND.name.en}`);

function normalizeTabName(value: string | number): AuthTabName {
  return value === "email-login" || value === "password" ? value : "phone";
}

function resetTabs() {
  userPickedTab = false;
  activeTab.value = preferredTab();
  activatedTabs.value = [activeTab.value];
}

function handleTabChange(value: string | number) {
  const nextTab = normalizeTabName(value);
  userPickedTab = true;
  activeTab.value = nextTab;
  if (!activatedTabs.value.includes(nextTab)) {
    activatedTabs.value = [...activatedTabs.value, nextTab];
  }
}

// 站点配置是异步取回的：首帧可能还没有 login 开关，取回后把默认 tab 纠正过来。
watch(preferredTab, (next) => {
  if (userPickedTab || next === activeTab.value) {
    return;
  }
  activeTab.value = next;
  if (!activatedTabs.value.includes(next)) {
    activatedTabs.value = [...activatedTabs.value, next];
  }
});

function isTabActivated(tabName: AuthTabName) {
  return activatedTabs.value.includes(tabName);
}

function handleModalVisibility(show: boolean) {
  uiStore.authModalOpen = show;
  if (!show) {
    resetTabs();
  }
}

function closeModal() {
  resetTabs();
  uiStore.closeAuthModal();
}
</script>

<template>
    <NModal :show="uiStore.authModalOpen" :mask-closable="true" @update:show="handleModalVisibility">
      <div class="mx-auto w-full max-w-[560px] px-4">
        <NCard
          :title="authTitle"
        :bordered="false"
        closable
        class="glass-thick"
        :style="{ '--n-color': 'var(--glass-thick-fill)', '--n-border-radius': 'var(--radius-xl)' }"
        @close="closeModal"
        @pointermove="handlePointerMove"
      >
        <div class="mb-5 text-sm muted">
          登录后即可继续创作、收藏、互动与查看消息。
        </div>

          <NTabs :value="activeTab" type="segment" animated @update:value="handleTabChange">
            <NTabPane name="phone" tab="手机号登录">
              <PhoneAuthForm v-if="isTabActivated('phone')" @success="closeModal" />
            </NTabPane>
            <NTabPane name="email-login" tab="邮箱登录">
              <EmailLoginForm v-if="isTabActivated('email-login')" @success="closeModal" />
            </NTabPane>
            <NTabPane name="password" tab="密码登录">
              <PasswordLoginForm v-if="isTabActivated('password')" @success="closeModal" />
            </NTabPane>
          </NTabs>
        </NCard>
    </div>
  </NModal>
</template>
