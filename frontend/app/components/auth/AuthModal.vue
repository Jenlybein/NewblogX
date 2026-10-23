<script setup lang="ts">
import { defineAsyncComponent, ref } from "vue";
import { NCard, NModal, NTabPane, NTabs } from "naive-ui";
import { useUiStore } from "~/stores/ui";

type AuthTabName = "password" | "email-login" | "register";

const PasswordLoginForm = defineAsyncComponent({
  loader: () => import("~/components/auth/PasswordLoginForm.vue"),
  suspensible: false,
});
const EmailLoginForm = defineAsyncComponent({
  loader: () => import("~/components/auth/EmailLoginForm.vue"),
  suspensible: false,
});
const EmailRegisterForm = defineAsyncComponent({
  loader: () => import("~/components/auth/EmailRegisterForm.vue"),
  suspensible: false,
});

const uiStore = useUiStore();
const activeTab = ref<AuthTabName>("password");
const activatedTabs = ref<AuthTabName[]>(["password"]);

function normalizeTabName(value: string | number): AuthTabName {
  return value === "email-login" || value === "register" ? value : "password";
}

function resetTabs() {
  activeTab.value = "password";
  activatedTabs.value = ["password"];
}

function handleTabChange(value: string | number) {
  const nextTab = normalizeTabName(value);
  activeTab.value = nextTab;
  if (!activatedTabs.value.includes(nextTab)) {
    activatedTabs.value = [...activatedTabs.value, nextTab];
  }
}

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
          title="加入 BlogX"
        :bordered="false"
        closable
        class="surface-card surface-card--strong"
        @close="closeModal"
      >
        <div class="mb-5 text-sm muted">
          登录后即可继续创作、收藏、互动与查看消息。
        </div>

          <NTabs :value="activeTab" type="segment" animated @update:value="handleTabChange">
            <NTabPane name="password" tab="密码登录">
              <PasswordLoginForm v-if="isTabActivated('password')" @success="closeModal" />
            </NTabPane>
            <NTabPane name="email-login" tab="邮箱登录">
              <EmailLoginForm v-if="isTabActivated('email-login')" @success="closeModal" />
            </NTabPane>
            <NTabPane name="register" tab="邮箱注册">
              <EmailRegisterForm v-if="isTabActivated('register')" @success="closeModal" />
            </NTabPane>
          </NTabs>
        </NCard>
    </div>
  </NModal>
</template>
