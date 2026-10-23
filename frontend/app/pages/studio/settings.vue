<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { NButton, NCard, NInput, NModal, NSelect, NSwitch, useMessage } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import { sendEmailCode } from "~/services/auth";
import { uploadImageByTask } from "~/services/image";
import { getTagOptions } from "~/services/search";
import {
  bindUserEmail,
  getMessagePreference,
  updateMessagePreference,
  updateUserProfile,
  renewPasswordByEmail,
} from "~/services/studio";
import { getSelfUserDetail } from "~/services/user";
import { resolveAvatarInitial, resolveAvatarUrl } from "~/utils/avatar";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const message = useMessage();
const authStore = useAuthStore();

const { data: profile, refresh: refreshProfile } = await useAsyncData("studio-settings-profile", () => getSelfUserDetail());
const { data: preference, refresh: refreshPreference } = await useAsyncData("studio-settings-preference", () => getMessagePreference());
const { data: tagOptions } = await useAsyncData("studio-settings-tag-options", () => getTagOptions().catch(() => []));

const profileForm = reactive({
  username: "",
  nickname: "",
  avatar: "",
  avatar_image_id: null as string | null,
  abstract: "",
  like_tag_ids: [] as string[],
  favorites_visibility: true,
  followers_visibility: true,
  fans_visibility: true,
  home_style_id: "",
});

const emailBindForm = reactive({
  email: "",
  email_id: "",
  email_code: "",
});

const passwordForm = reactive({
  old_password: "",
  new_password: "",
});

const preferenceForm = reactive({
  digg_notice_enabled: true,
  comment_notice_enabled: true,
  favor_notice_enabled: true,
  private_chat_notice_enabled: true,
});

const avatarFileInputRef = ref<HTMLInputElement | null>(null);
const avatarUploading = reactive({
  pending: false,
  stage: "",
});
const avatarDirty = ref(false);
const showEmailModal = ref(false);
const showPasswordModal = ref(false);

watch(
  () => profile.value,
  (value) => {
    if (!value) return;
    profileForm.username = value.username ?? "";
    profileForm.nickname = value.nickname ?? "";
    profileForm.avatar = value.avatar ?? "";
    profileForm.avatar_image_id = value.avatar_image_id ?? null;
    avatarDirty.value = false;
    profileForm.abstract = value.abstract ?? "";
    profileForm.like_tag_ids = [...(value.like_tag_ids ?? [])];
    profileForm.favorites_visibility = value.favorites_visibility ?? true;
    profileForm.followers_visibility = value.followers_visibility ?? true;
    profileForm.fans_visibility = value.fans_visibility ?? true;
    profileForm.home_style_id = value.home_style_id ?? "";
  },
  { immediate: true },
);

watch(
  () => preference.value,
  (value) => {
    if (!value) return;
    preferenceForm.digg_notice_enabled = value.digg_notice_enabled;
    preferenceForm.comment_notice_enabled = value.comment_notice_enabled;
    preferenceForm.favor_notice_enabled = value.favor_notice_enabled;
    preferenceForm.private_chat_notice_enabled = value.private_chat_notice_enabled;
  },
  { immediate: true },
);

const enabledTagIdSet = computed(() => {
  const set = new Set<string>();
  for (const option of tagOptions.value ?? []) {
    const raw = option?.value;
    if (raw == null) continue;
    const id = String(raw).trim();
    if (!id) continue;
    set.add(id);
  }
  return set;
});

const currentEmail = computed(() => {
  const maybeProfileEmail = (profile.value as { email?: string } | null)?.email ?? "";
  const maybeAuthEmail = (authStore.currentUser as { email?: string } | null)?.email ?? "";
  const candidates = [emailBindForm.email, maybeProfileEmail, maybeAuthEmail];
  for (const item of candidates) {
    const normalized = item.trim();
    if (normalized && normalized.includes("@")) {
      return normalized;
    }
  }
  return "";
});

const hasBoundEmail = computed(() => Boolean(currentEmail.value));

const maskedEmailText = computed(() => {
  if (!currentEmail.value) {
    return "********(填入真实邮箱)";
  }

  const [localPart, domain = ""] = currentEmail.value.split("@");
  if (!localPart) {
    return "********(填入真实邮箱)";
  }

  const visiblePrefix = localPart.slice(0, Math.min(2, localPart.length));
  return `${visiblePrefix}${"*".repeat(6)}@${domain}`;
});

const hasBoundPassword = computed(() => {
  const user = profile.value as { has_password?: boolean; password_bound?: boolean } | null;
  if (typeof user?.has_password === "boolean") {
    return user.has_password;
  }
  if (typeof user?.password_bound === "boolean") {
    return user.password_bound;
  }
  return true;
});

const maskedPasswordText = computed(() => (hasBoundPassword.value ? "******" : "******(未绑定)"));
const profileAvatarPreview = computed(() => resolveAvatarUrl(profileForm.avatar));
const profileAvatarInitial = computed(() => resolveAvatarInitial(profileForm.nickname || profileForm.username, "我"));

async function saveProfile() {
  if (avatarUploading.pending) {
    message.warning("头像仍在上传中，请稍后再保存资料。");
    return;
  }

  try {
    const trimmedAvatarImageId = (profileForm.avatar_image_id ?? "").trim();
    const hasAvatarImageId = Boolean(trimmedAvatarImageId);
    if (hasAvatarImageId && !/^\d+$/.test(trimmedAvatarImageId)) {
      message.error("头像标识格式不合法，请重新上传头像后再保存。");
      return;
    }

    const normalizedLikeTagIds = profileForm.like_tag_ids
      .map((id) => String(id).trim())
      .filter((id) => Boolean(id) && enabledTagIdSet.value.has(id));
    if (normalizedLikeTagIds.length !== profileForm.like_tag_ids.length) {
      profileForm.like_tag_ids = normalizedLikeTagIds;
      message.warning("已自动移除不存在或停用的偏好标签，请确认后再次保存。");
      return;
    }

    const payload: Parameters<typeof updateUserProfile>[0] = {
      username: profileForm.username.trim() || null,
      nickname: profileForm.nickname.trim() || null,
      abstract: profileForm.abstract.trim() || null,
      like_tag_ids: normalizedLikeTagIds,
      favorites_visibility: profileForm.favorites_visibility,
      followers_visibility: profileForm.followers_visibility,
      fans_visibility: profileForm.fans_visibility,
      home_style_id: profileForm.home_style_id || null,
    };

    if (hasAvatarImageId) {
      payload.avatar_image_id = trimmedAvatarImageId;
    } else if (avatarDirty.value) {
      payload.avatar_image_id = null;
    }

    await updateUserProfile(payload);
    await Promise.all([refreshProfile(), authStore.fetchCurrentUser()]);
    avatarDirty.value = false;
    message.success("个人资料已更新");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "保存资料失败");
  }
}

function openAvatarPicker() {
  if (avatarUploading.pending) {
    return;
  }
  avatarFileInputRef.value?.click();
}

function openEmailModalDialog() {
  emailBindForm.email = currentEmail.value || emailBindForm.email;
  emailBindForm.email_code = "";
  emailBindForm.email_id = "";
  showEmailModal.value = true;
}

function openPasswordModalDialog() {
  passwordForm.old_password = "";
  passwordForm.new_password = "";
  showPasswordModal.value = true;
}

function clearAvatar() {
  profileForm.avatar = "";
  profileForm.avatar_image_id = null;
  avatarDirty.value = true;
  avatarUploading.stage = "";
  if (avatarFileInputRef.value) {
    avatarFileInputRef.value.value = "";
  }
}

function mapAvatarUploadStage(stage: "hashing" | "creating_task" | "uploading_to_qiniu" | "polling_status") {
  const stageMap = {
    hashing: "正在计算文件指纹…",
    creating_task: "正在创建上传任务…",
    uploading_to_qiniu: "正在上传头像…",
    polling_status: "正在确认上传状态…",
  } as const;
  avatarUploading.stage = stageMap[stage];
}

async function handleAvatarFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) {
    return;
  }

  if (!file.type.startsWith("image/")) {
    message.warning("请选择图片文件作为头像");
    input.value = "";
    return;
  }

  avatarUploading.pending = true;
  avatarUploading.stage = "开始上传头像…";
  try {
    const uploadResult = await uploadImageByTask(file, {
      onStage: mapAvatarUploadStage,
    });
    if (!uploadResult.image_id || !uploadResult.url) {
      throw new Error("上传成功但缺少图片标识");
    }
    profileForm.avatar = uploadResult.url;
    profileForm.avatar_image_id = uploadResult.image_id;
    avatarDirty.value = true;
    avatarUploading.stage = "头像上传完成";
    message.success("头像上传成功");
  } catch (error) {
    avatarUploading.stage = "";
    message.error(error instanceof Error ? error.message : "头像上传失败");
  } finally {
    avatarUploading.pending = false;
    input.value = "";
  }
}

async function sendBindCode() {
  try {
    const payload = await sendEmailCode({
      email: emailBindForm.email.trim(),
      type: 4,
    });
    emailBindForm.email_id = payload.email_id;
    message.success("邮箱验证码已发送");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "发送验证码失败");
  }
}

async function submitBindEmail() {
  try {
    await bindUserEmail({
      email_id: emailBindForm.email_id,
      email_code: emailBindForm.email_code.trim(),
    });
    message.success("邮箱已绑定");
    emailBindForm.email_code = "";
    showEmailModal.value = false;
    await Promise.all([refreshProfile(), authStore.fetchCurrentUser()]);
  } catch (error) {
    message.error(error instanceof Error ? error.message : "绑定邮箱失败");
  }
}

async function submitPasswordReset() {
  try {
    await renewPasswordByEmail({
      old_password: passwordForm.old_password,
      new_password: passwordForm.new_password,
    });
    passwordForm.old_password = "";
    passwordForm.new_password = "";
    showPasswordModal.value = false;
    message.success("密码已更新，请留意旧登录态可能会失效");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "更新密码失败");
  }
}

async function savePreference() {
  try {
    await updateMessagePreference({ ...preferenceForm });
    await refreshPreference();
    message.success("消息偏好已保存");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "保存消息偏好失败");
  }
}

useSeoMeta({
  title: "个人中心 - 账号设置",
});
</script>

<template>
  <div class="page-stack">
    <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
      <section class="space-y-5">
        <NCard class="studio-list-card" :bordered="false">
          <div class="studio-toolbar">
            <div>
              <div class="eyebrow">Profile</div>
              <h2 class="section-title mt-2">个人资料</h2>
            </div>
            <NButton type="primary" :disabled="avatarUploading.pending" @click="saveProfile()">保存资料</NButton>
          </div>

          <div class="mt-5 grid gap-4 md:grid-cols-2">
            <label class="space-y-2">
              <span class="text-sm font-medium">用户名</span>
              <NInput v-model:value="profileForm.username" maxlength="20" placeholder="输入用户名…" />
            </label>
            <label class="space-y-2">
              <span class="text-sm font-medium">昵称</span>
              <NInput v-model:value="profileForm.nickname" maxlength="20" placeholder="输入昵称…" />
            </label>
            <label class="space-y-2 md:col-span-2">
              <span class="text-sm font-medium">头像上传</span>
              <div class="rounded-[18px] border px-4 py-4 bg-white/40">
                <input
                  ref="avatarFileInputRef"
                  type="file"
                  accept="image/*"
                  class="hidden"
                  @change="handleAvatarFileChange" />

                <div class="flex min-h-[92px] flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                  <div class="flex items-center gap-3">
                    <AppAvatar :size="56" :src="profileAvatarPreview" :name="profileAvatarInitial" :fallback="profileAvatarInitial" />
                    <div class="space-y-1">
                      <p class="text-sm font-medium">更新你的个人头像</p>
                      <p class="text-xs muted">支持 JPG / PNG / JPEG，建议方形图片，大小 5MB 以内</p>
                    </div>
                  </div>

                  <div class="flex flex-wrap gap-2 sm:justify-end">
                    <NButton quaternary :loading="avatarUploading.pending" @click="openAvatarPicker()">
                      {{ profileForm.avatar ? "重新上传头像" : "选择头像图片" }}
                    </NButton>
                    <NButton v-if="profileForm.avatar || profileForm.avatar_image_id" quaternary @click="clearAvatar()">
                      移除头像
                    </NButton>
                  </div>
                </div>

                <p v-if="avatarUploading.stage" class="mt-3 text-xs muted">{{ avatarUploading.stage }}</p>
              </div>
            </label>
            <label class="space-y-2 md:col-span-2">
              <span class="text-sm font-medium">个人简介</span>
              <NInput
                v-model:value="profileForm.abstract"
                type="textarea"
                :autosize="{ minRows: 4, maxRows: 6 }"
                maxlength="120"
                placeholder="介绍你的创作方向、关注主题或个人偏好…"
              />
            </label>
            <label class="space-y-2 md:col-span-2">
              <span class="text-sm font-medium">偏好标签</span>
              <NSelect
                v-model:value="profileForm.like_tag_ids"
                multiple
                filterable
                clearable
                max-tag-count="responsive"
                :options="tagOptions || []"
                placeholder="选择你感兴趣的标签，提交时会走 like_tag_ids"
              />
            </label>
            <label class="space-y-2 md:col-span-2 lg:max-w-[320px]">
              <span class="text-sm font-medium">主页样式 ID</span>
              <NInput v-model:value="profileForm.home_style_id" placeholder="例如：1、2、3" />
            </label>
          </div>
        </NCard>

        <NCard class="studio-list-card" :bordered="false">
          <div class="eyebrow">Bind</div>
          <h2 class="section-title mt-2">绑定信息</h2>

          <div class="mt-5 space-y-3">
            <div class="flex items-center justify-between rounded-[18px] border px-4 py-3">
              <div class="flex items-center gap-4">
                <span class="text-sm font-medium">邮箱</span>
                <span class="text-sm muted">{{ maskedEmailText }}</span>
              </div>
              <NButton text type="primary" @click="openEmailModalDialog()">
                {{ hasBoundEmail ? "修改邮箱" : "绑定邮箱" }}
              </NButton>
            </div>
            <div class="flex items-center justify-between rounded-[18px] border px-4 py-3">
              <div class="flex items-center gap-4">
                <span class="text-sm font-medium">密码</span>
                <span class="text-sm muted">{{ maskedPasswordText }}</span>
              </div>
              <NButton text type="primary" @click="openPasswordModalDialog()">
                {{ hasBoundPassword ? "修改密码" : "绑定密码" }}
              </NButton>
            </div>
          </div>
        </NCard>
      </section>

      <section class="space-y-5">
        <NCard class="studio-list-card" :bordered="false">
          <div class="eyebrow">Preference</div>
          <h2 class="section-title mt-2">消息偏好</h2>

          <div class="mt-5 space-y-3">
            <div class="flex items-center justify-between rounded-[18px] border px-4 py-3">
              <span>点赞提醒</span>
              <NSwitch v-model:value="preferenceForm.digg_notice_enabled" />
            </div>
            <div class="flex items-center justify-between rounded-[18px] border px-4 py-3">
              <span>评论提醒</span>
              <NSwitch v-model:value="preferenceForm.comment_notice_enabled" />
            </div>
            <div class="flex items-center justify-between rounded-[18px] border px-4 py-3">
              <span>收藏提醒</span>
              <NSwitch v-model:value="preferenceForm.favor_notice_enabled" />
            </div>
            <div class="flex items-center justify-between rounded-[18px] border px-4 py-3">
              <span>私信提醒</span>
              <NSwitch v-model:value="preferenceForm.private_chat_notice_enabled" />
            </div>
          </div>
          <div class="mt-4">
            <NButton type="primary" @click="savePreference()">保存偏好</NButton>
          </div>
        </NCard>

        <NCard class="studio-list-card" :bordered="false">
          <div class="eyebrow">Visibility</div>
          <h2 class="section-title mt-2">可见性</h2>

          <div class="mt-5 space-y-3">
            <div class="flex items-center justify-between rounded-[18px] border px-4 py-3">
              <span>收藏夹可见</span>
              <NSwitch v-model:value="profileForm.favorites_visibility" />
            </div>
            <div class="flex items-center justify-between rounded-[18px] border px-4 py-3">
              <span>关注列表可见</span>
              <NSwitch v-model:value="profileForm.followers_visibility" />
            </div>
            <div class="flex items-center justify-between rounded-[18px] border px-4 py-3">
              <span>粉丝列表可见</span>
              <NSwitch v-model:value="profileForm.fans_visibility" />
            </div>
          </div>
        </NCard>
      </section>
    </div>

    <NModal
      v-model:show="showEmailModal"
      preset="card"
      title="修改邮箱"
      class="w-[min(92vw,520px)]"
      :mask-closable="false">
      <div class="space-y-4">
        <label class="space-y-2 block">
          <span class="text-sm font-medium">邮箱地址</span>
          <NInput v-model:value="emailBindForm.email" placeholder="name@example.com" />
        </label>
        <div class="flex flex-wrap gap-3">
          <NButton quaternary @click="sendBindCode()">发送邮箱验证码</NButton>
          <span v-if="emailBindForm.email_id" class="glass-badge">email_id 已生成</span>
        </div>
        <label class="space-y-2 block">
          <span class="text-sm font-medium">邮箱验证码</span>
          <NInput v-model:value="emailBindForm.email_code" maxlength="8" placeholder="输入邮箱验证码…" />
        </label>
        <div class="flex justify-end gap-2 pt-1">
          <NButton quaternary @click="showEmailModal = false">取消</NButton>
          <NButton type="primary" @click="submitBindEmail()">{{ hasBoundEmail ? "确认修改" : "确认绑定" }}</NButton>
        </div>
      </div>
    </NModal>

    <NModal
      v-model:show="showPasswordModal"
      preset="card"
      title="修改密码"
      class="w-[min(92vw,520px)]"
      :mask-closable="false">
      <div class="space-y-4">
        <p class="text-sm leading-6 muted">密码接口当前要求提交旧密码和新密码。</p>
        <label class="space-y-2 block">
          <span class="text-sm font-medium">旧密码</span>
          <NInput v-model:value="passwordForm.old_password" type="password" show-password-on="click" />
        </label>
        <label class="space-y-2 block">
          <span class="text-sm font-medium">新密码</span>
          <NInput v-model:value="passwordForm.new_password" type="password" show-password-on="click" />
        </label>
        <div class="flex justify-end gap-2 pt-1">
          <NButton quaternary @click="showPasswordModal = false">取消</NButton>
          <NButton type="primary" @click="submitPasswordReset()">{{ hasBoundPassword ? "确认修改" : "确认绑定" }}</NButton>
        </div>
      </div>
    </NModal>
  </div>
</template>
