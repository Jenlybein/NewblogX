<script setup lang="ts">
import ChipSelect from "~/components/common/ChipSelect.vue";
import { computed, reactive, ref } from "vue";
import { NButton, NCard, NInput, NModal, NSwitch, useMessage } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import ImageCropDialog from "~/components/studio/ImageCropDialog.vue";
import { bindPhone, sendEmailCode, sendSmsCode, setAccountPassword } from "~/services/auth";
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
import { useImageCropUpload, type ImageFramingRequest } from "~/composables/useImageCropUpload";
import {
  AVATAR_RATIO_TEXT,
  AVATAR_SHAPE_OPTIONS,
  AVATAR_SHAPE_TARGET,
  resolveAvatarInitial,
  resolveAvatarUrl,
  shapeAvatar,
  shapeAvatarWithFraming,
} from "~/utils/avatar";
import { isImageFile, measureImage, type ShapeFramingState } from "~/utils/imageShaping";
import { buildPrivacyUpdatePayload, buildProfileUpdatePayload, buildVisibilityUpdatePayload } from "~/utils/studioSettings";
import { errorMessage } from "~/utils/errorMessage";
import { useVerificationCountdown } from "~/composables/useVerificationCountdown";
import { useCaptchaProof } from "~/composables/useCaptchaProof";
import ImageCaptchaField from "~/components/auth/ImageCaptchaField.vue";

definePageMeta({
  layout: "studio",
  middleware: "auth",
});

const message = useMessage();
const authStore = useAuthStore();
const route = useRoute();

/**
 * 账号设置的五个分栏（票 355）。
 *
 * 用 `?section=` 而不是五个路由文件：分栏共享同一份数据加载（资料 / 偏好 /
 * 标签选项），拆成五个页面就会各自拉一遍，且"保存后刷新"要重写五份。
 * 顺序即侧栏子项顺序，也即页面上的 tab 顺序。
 */
const sectionTabs = [
  { key: "profile", label: "个人资料" },
  { key: "security", label: "账号安全" },
  { key: "notify", label: "消息偏好" },
  { key: "visibility", label: "可见性" },
  { key: "privacy", label: "隐私" },
] as const;

type SettingsSection = (typeof sectionTabs)[number]["key"];

/** 当前分栏：不认识的 section 一律回落到「个人资料」，不显示空白页。 */
const activeSection = computed<SettingsSection>(() => {
  const raw = route.query.section;
  const value = typeof raw === "string" ? raw : "profile";
  return (sectionTabs.some(tab => tab.key === value) ? value : "profile") as SettingsSection;
});

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
  // 允许陌生人给我发私信（后端两级判定的个人一级，默认开启）。
  stranger_msg_enabled: true,
});

const emailBindForm = reactive({
  email: "",
  email_id: "",
  email_code: "",
});

/** 绑定手机号：号码与短信验证码。只在"尚未绑定"时使用。 */
const phoneBindForm = reactive({
  phone: "",
  code: "",
});

const passwordForm = reactive({
  old_password: "",
  new_password: "",
  // 仅在"还没有密码"时使用：设置首个密码要对绑定手机号再做一次短信复验。
  phone: "",
  sms_code: "",
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
const showPhoneModal = ref(false);
const showPasswordModal = ref(false);

/**
 * 裁剪窗请求（票 215）：非 null 表示头像比例不合规，等作者选区域。
 * 这段时间没有上传、没有改 profileForm；取消就保留原头像。
 */
const avatarCropRequest = ref<ImageFramingRequest | null>(null);
/** 待裁剪原图的本地预览地址（弹窗里可拖动的那张图）。 */
const avatarCropPreviewUrl = ref("");

function createPreviewUrl(blob: Blob): string {
  try {
    return typeof URL.createObjectURL === "function" ? URL.createObjectURL(blob) : "";
  } catch {
    return "";
  }
}

function revokePreviewUrl(url: string) {
  if (!url) return;
  try {
    if (typeof URL.revokeObjectURL === "function") URL.revokeObjectURL(url);
  } catch {
    // 已释放或环境不支持：忽略。
  }
}

function resetAvatarCrop() {
  const url = avatarCropPreviewUrl.value;
  avatarCropPreviewUrl.value = "";
  avatarCropRequest.value = null;
  revokePreviewUrl(url);
}

/**
 * 头像的上传顺序与封面同源（`useImageCropUpload`）：
 * 合规 1:1 → 居中定形 256×256 PNG 直接上传；不合规 → 只打开裁剪窗。
 */
const avatarCropFlow = useImageCropUpload({
  options: AVATAR_SHAPE_OPTIONS,
  measure: (file) => measureImage(file),
  shape: async (file, options) => {
    const blob = options.framing
      ? await shapeAvatarWithFraming(file, options.framing)
      : await shapeAvatar(file);
    return {
      file: new File([blob], "avatar.png", { type: blob.type }),
      previewUrl: "",
      size: AVATAR_SHAPE_OPTIONS.target,
    };
  },
  requestFraming: (request) => {
    avatarCropRequest.value = request;
    avatarCropPreviewUrl.value = createPreviewUrl(request.file);
    avatarUploading.stage = `图片比例不符合 ${AVATAR_RATIO_TEXT}，请选择要露出的区域`;
  },
  upload: async (shaped) => {
    const uploadResult = await uploadImageByTask(shaped.file, {
      onStage: mapAvatarUploadStage,
      purpose: "avatar",
    });
    if (!uploadResult.image_id || !uploadResult.url) {
      throw new Error("上传成功但缺少图片标识");
    }
    profileForm.avatar = uploadResult.url;
    profileForm.avatar_image_id = uploadResult.image_id;
    avatarDirty.value = true;
    avatarUploading.stage = "头像上传完成";
    message.success("头像上传成功");
  },
});

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
    // 默认开启：老账号没有这条配置行时后端也按允许处理，前端不能显示成关闭。
    profileForm.stranger_msg_enabled = value.stranger_msg_enabled ?? true;
  },
  { immediate: true },
);

watch(
  () => preference.value,
  (value) => {
    if (!value) return;
    preferenceForm.digg_notice_enabled = value.digg_notice_enabled ?? false;
    preferenceForm.comment_notice_enabled = value.comment_notice_enabled ?? false;
    preferenceForm.favor_notice_enabled = value.favor_notice_enabled ?? false;
    preferenceForm.private_chat_notice_enabled = value.private_chat_notice_enabled ?? false;
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

const maskedPasswordText = computed(() => (hasBoundPassword.value ? "******" : "******(未设置)"));

/** 当前账号绑定的手机号（完整号码，只出现在"我自己"的详情里）。 */
const currentPhone = computed(() => {
  const user = profile.value as { phone?: string | null } | null;
  return (user?.phone ?? "").trim();
});
const hasBoundPhone = computed(() => currentPhone.value.length > 0);
const maskedPhoneText = computed(() => {
  const phone = currentPhone.value;
  if (!phone) {
    return "********(未绑定)";
  }
  if (phone.length !== 11) {
    return "***";
  }
  return `${phone.slice(0, 3)}****${phone.slice(7)}`;
});

/** 设置首个密码时的短信验证码倒计时。 */
const {
  counting: passwordSmsLocked,
  sendLabel: passwordSmsLabel,
  start: startPasswordSmsCountdown,
  reset: resetPasswordSmsCountdown,
} = useVerificationCountdown();

/** 绑定手机号时的短信验证码倒计时（与设密码那条各算各的：两条通道互不影响）。 */
const {
  counting: phoneBindSmsLocked,
  sendLabel: phoneBindSmsLabel,
  start: startPhoneBindSmsCountdown,
  reset: resetPhoneBindSmsCountdown,
} = useVerificationCountdown();

/**
 * 图形验证码：本页两条发码通道（绑定邮箱、设密码复验）后端都挂了同一个中间件，
 * 所以共用一份状态——两个弹窗不会同时打开，一次只服务一条。
 */
const {
  enabled: captchaEnabled,
  image: captchaImage,
  code: captchaCode,
  loading: captchaLoading,
  missing: captchaMissing,
  refresh: refreshCaptcha,
  payload: captchaPayload,
} = useCaptchaProof();
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

    const payload = buildProfileUpdatePayload(
      {
        username: profileForm.username,
        nickname: profileForm.nickname,
        abstract: profileForm.abstract,
        favorites_visibility: profileForm.favorites_visibility,
        followers_visibility: profileForm.followers_visibility,
        fans_visibility: profileForm.fans_visibility,
      },
      {
        likeTagIds: normalizedLikeTagIds,
        // 三种头像状态：上传了新图 → 传新标识；点过"移除" → null；否则不传（不改）。
        ...(hasAvatarImageId
          ? { avatarImageId: trimmedAvatarImageId }
          : avatarDirty.value
            ? { avatarImageId: null }
            : {}),
      },
    );

    await updateUserProfile(payload);
    await Promise.all([refreshProfile(), authStore.fetchCurrentUser()]);
    avatarDirty.value = false;
    message.success("个人资料已更新");
  } catch (error) {
    message.error(errorMessage(error, "保存资料失败"));
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
  passwordForm.phone = currentPhone.value;
  passwordForm.sms_code = "";
  resetPasswordSmsCountdown();
  showPasswordModal.value = true;
}

/**
 * 给"当前密码"弹窗发一条短信验证码。
 *
 * 只有"还没有密码"的账号会走到这里：他们无法提供旧密码，所以用绑定手机号的
 * 短信复验来证明身份（后端 SetPassword 会核对号码是否与绑定的一致）。
 */
async function sendPasswordCode() {
  if (!hasBoundPhone.value) {
    message.warning("当前账号还没有绑定手机号，无法用短信验证身份");
    return;
  }
  if (captchaMissing.value) {
    message.warning("请输入图形验证码");
    return;
  }

  try {
    await sendSmsCode({
      phone: currentPhone.value,
      purpose: "passwordProof",
      captcha: captchaPayload(),
    });
    startPasswordSmsCountdown();
    void refreshCaptcha();
    message.success("验证码已发送");
  } catch (error) {
    message.error(errorMessage(error, "发送验证码失败"));
  }
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

/**
 * 选完头像后（票 215）：量原图尺寸 → 判 1:1 比例。
 * 相差 ≤1% → 自动居中定形成 256×256 PNG 并上传；相差 >1% → 只打开裁剪窗，
 * 不产出文件、不上传；取消即保留原头像。
 */
async function handleAvatarFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) {
    return;
  }

  if (!isImageFile(file)) {
    message.warning("请选择图片文件作为头像");
    input.value = "";
    return;
  }

  resetAvatarCrop();
  avatarUploading.pending = true;
  avatarUploading.stage = "正在检查图片比例…";
  try {
    await avatarCropFlow.select(file);
  } catch (error) {
    avatarUploading.stage = "";
    message.error(errorMessage(error, "头像上传失败"));
  } finally {
    avatarUploading.pending = false;
    input.value = "";
  }
}

/** 裁剪窗确认：按选定区域定形成新的 256×256 PNG 再上传（不存裁剪参数）。 */
async function confirmAvatarCrop(framing: ShapeFramingState) {
  const request = avatarCropRequest.value;
  if (!request) {
    return;
  }
  resetAvatarCrop();
  avatarUploading.pending = true;
  avatarUploading.stage = "正在按选定区域定形头像…";
  try {
    await avatarCropFlow.confirm(request, framing);
  } catch (error) {
    avatarUploading.stage = "";
    message.error(errorMessage(error, "头像上传失败"));
  } finally {
    avatarUploading.pending = false;
    if (avatarFileInputRef.value) {
      avatarFileInputRef.value.value = "";
    }
  }
}

/** 裁剪窗取消：中止本次上传；原头像（profileForm.avatar / avatar_image_id）保持不变。 */
function cancelAvatarCrop() {
  resetAvatarCrop();
  avatarUploading.stage = "已取消本次上传，保留原头像";
}

async function sendBindCode() {
  if (captchaMissing.value) {
    message.warning("请输入图形验证码");
    return;
  }

  try {
    const payload = await sendEmailCode({
      email: emailBindForm.email.trim(),
      purpose: "bindEmail",
      captcha: captchaPayload(),
    });
    emailBindForm.email_id = payload.id;
    // 图形码一次性：用过就换一张。
    void refreshCaptcha();
    message.success("邮箱验证码已发送");
  } catch (error) {
    message.error(errorMessage(error, "发送验证码失败"));
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
    message.error(errorMessage(error, "绑定邮箱失败"));
  }
}

/**
 * 发送绑定手机号的短信验证码。
 *
 * 走的是登录那条通道（purpose=login）：后端核对的是"这个号码现在在你手上"，
 * 与拿它登录是同一种证明，没有为绑定另造一个号位。
 */
async function sendPhoneBindCode() {
  const phone = phoneBindForm.phone.trim();
  if (!/^1[3-9]\d{9}$/.test(phone)) {
    message.warning("请输入 11 位手机号");
    return;
  }
  if (captchaMissing.value) {
    message.warning("请输入图形验证码");
    return;
  }

  try {
    await sendSmsCode({ phone, purpose: "login", captcha: captchaPayload() });
    // 图形码一次性：用过就换一张。
    void refreshCaptcha();
    startPhoneBindSmsCountdown();
    message.success("短信验证码已发送");
  } catch (error) {
    message.error(errorMessage(error, "发送验证码失败"));
  }
}

async function submitBindPhone() {
  try {
    await bindPhone({ phone: phoneBindForm.phone.trim(), code: phoneBindForm.code.trim() });
    message.success("手机号已绑定");
    phoneBindForm.code = "";
    showPhoneModal.value = false;
    resetPhoneBindSmsCountdown();
    await Promise.all([refreshProfile(), authStore.fetchCurrentUser()]);
  } catch (error) {
    message.error(errorMessage(error, "绑定手机号失败"));
  }
}

function openPhoneModal() {
  phoneBindForm.phone = "";
  phoneBindForm.code = "";
  resetPhoneBindSmsCountdown();
  showPhoneModal.value = true;
  // 站点开了图形验证码就先把图取回来，省得用户点了发码才等。
  if (captchaEnabled.value) void refreshCaptcha();
}

async function submitPasswordReset() {
  try {
    if (hasBoundPassword.value) {
      await renewPasswordByEmail({
        old_password: passwordForm.old_password,
        new_password: passwordForm.new_password,
      });
      passwordForm.old_password = "";
      message.success("密码已更新，请留意旧登录态可能会失效");
    } else {
      if (!passwordForm.sms_code.trim()) {
        message.warning("请先输入短信验证码");
        return;
      }
      // 首次设置密码：不需要旧密码，但要用绑定手机号的短信复验证明身份。
      await setAccountPassword({
        phone: passwordForm.phone,
        code: passwordForm.sms_code.trim(),
        new_password: passwordForm.new_password,
      });
      passwordForm.sms_code = "";
      message.success("密码已设置，现在可以用「用户名 + 密码」登录");
    }
    passwordForm.new_password = "";
    showPasswordModal.value = false;
    // 刷新详情：has_password 变了，"设置密码/修改密码"的文案要跟着变。
    await Promise.all([refreshProfile(), authStore.fetchCurrentUser()]);
  } catch (error) {
    message.error(errorMessage(error, hasBoundPassword.value ? "更新密码失败" : "设置密码失败"));
  }
}

/**
 * 可见性独立保存（票 355）。
 *
 * 之前三个开关混在"保存资料"里：用户只想改一个可见性，却会把整张资料卡
 * （用户名、昵称、简介、偏好标签）一起提交一遍。独立之后提交的只有这三个字段。
 */
async function saveVisibility() {
  try {
    await updateUserProfile(buildVisibilityUpdatePayload({
      favorites_visibility: profileForm.favorites_visibility,
      followers_visibility: profileForm.followers_visibility,
      fans_visibility: profileForm.fans_visibility,
    }));
    await refreshProfile();
    message.success("可见性设置已更新");
  } catch (error) {
    message.error(errorMessage(error, "保存可见性失败"));
  }
}

async function savePreference() {
  try {
    await updateMessagePreference({ ...preferenceForm });
    await refreshPreference();
    message.success("消息偏好已保存");
  } catch (error) {
    message.error(errorMessage(error, "保存消息偏好失败"));
  }
}

/**
 * 保存隐私开关。
 *
 * 只提交 `stranger_msg_enabled` 这一项，走与"保存资料"同一个 `PUT /users/info`；
 * 后端把个人开关与站点总闸取交集，保存后立刻影响下一条陌生私信的判定。
 *
 * 这是该字段**唯一**的提交路径（评审票 248 / P1-4）：资料卡不再附带它，
 * 否则两张卡会争同一个字段。
 */
async function savePrivacy() {
  try {
    await updateUserProfile(buildPrivacyUpdatePayload(profileForm.stranger_msg_enabled));
    await refreshProfile();
    message.success("隐私设置已保存");
  } catch (error) {
    message.error(errorMessage(error, "保存隐私设置失败"));
  }
}

useSeoMeta({
  title: "个人中心 - 账号设置",
});
useNoIndexPage();
</script>

<template>
  <div class="page-stack">
    <!-- 单列 + 分栏（票 355）：一屏只看一个分栏，切换靠侧栏子项与页内 tab。
         之前是两列各塞几张卡，同一件事（我的资料）被拆到左右两栏，眼睛来回跳。 -->
    <nav class="settings-tabs" aria-label="账号设置分栏">
      <NuxtLink
        v-for="tab in sectionTabs"
        :key="tab.key"
        :to="`/studio/settings?section=${tab.key}`"
        class="settings-tabs__tab"
        :class="{ 'is-active': activeSection === tab.key }"
        :data-role="`settings-tab-${tab.key}`">
        {{ tab.label }}
      </NuxtLink>
    </nav>

    <!-- 个人资料 -->
    <NCard v-if="activeSection === 'profile'" class="studio-list-card" :bordered="false">
      <div class="studio-toolbar">
        <div>
          <div class="eyebrow">Profile</div>
          <h2 class="section-title mt-2">个人资料</h2>
        </div>
        <NButton type="primary" :disabled="avatarUploading.pending" data-role="settings-save-profile" @click="saveProfile()">
          保存资料
        </NButton>
      </div>

      <div class="mt-4">
        <div class="settings-row">
          <span class="settings-row__label">用户名</span>
          <div class="settings-row__control">
            <NInput v-model:value="profileForm.username" maxlength="20" placeholder="输入用户名…" />
          </div>
        </div>

        <div class="settings-row">
          <span class="settings-row__label">昵称</span>
          <div class="settings-row__control">
            <NInput v-model:value="profileForm.nickname" maxlength="20" placeholder="输入昵称…" />
          </div>
        </div>

        <div class="settings-row settings-row--switch">
          <span class="settings-row__label">头像</span>
          <div class="settings-row__control">
            <input
              ref="avatarFileInputRef"
              type="file"
              accept="image/*"
              class="hidden"
              @change="handleAvatarFileChange" />

            <div class="flex min-h-[72px] flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
              <div class="flex items-center gap-3">
                <AppAvatar :size="56" :src="profileAvatarPreview" :name="profileAvatarInitial" :fallback="profileAvatarInitial" />
                <div class="space-y-1">
                  <p class="text-sm font-medium">更新你的个人头像</p>
                  <p class="text-xs muted">支持 JPG / PNG / WebP，会上传前居中裁成方形并压到 256×256</p>
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
        </div>

        <div class="settings-row settings-row--switch">
          <span class="settings-row__label">个人简介</span>
          <div class="settings-row__control">
            <NInput
              v-model:value="profileForm.abstract"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 6 }"
              maxlength="120"
              placeholder="介绍你的创作方向、关注主题或个人偏好…"
            />
          </div>
        </div>

        <div class="settings-row settings-row--switch">
          <span class="settings-row__label">偏好标签</span>
          <div class="settings-row__control">
            <ChipSelect
              v-model="profileForm.like_tag_ids"
              multiple
              :max-tag-count="5"
              :options="tagOptions || []"
              placeholder="选择你感兴趣的标签，提交时会走 like_tag_ids"
              data-role="settings-like-tags"
            />
            <p class="settings-row__hint">读者侧会用这些标签给你推荐相关内容。</p>
          </div>
        </div>
      </div>
    </NCard>

    <!-- 账号安全 -->
    <NCard v-if="activeSection === 'security'" class="studio-list-card" :bordered="false">
      <div class="eyebrow">Security</div>
      <h2 class="section-title mt-2">账号安全</h2>
      <p class="mt-1 text-sm muted">邮箱与密码都走独立弹窗：改它们要验证码 / 旧密码，不适合塞在行内。</p>

      <div class="mt-4">
        <div class="settings-row">
          <span class="settings-row__label">手机号</span>
          <div class="settings-row__control flex items-center justify-between gap-3">
            <span class="text-sm muted">{{ maskedPhoneText }}</span>
            <!-- 手机号是登录主标识：未绑定时给一个入口，已绑定时只说明用途（换绑本期不做）。 -->
            <NButton
              v-if="!hasBoundPhone"
              text
              type="primary"
              data-role="settings-bind-phone"
              @click="openPhoneModal()"
            >
              绑定手机号
            </NButton>
            <span v-else class="text-xs muted">用于登录与找回</span>
          </div>
        </div>

        <div class="settings-row">
          <span class="settings-row__label">邮箱</span>
          <div class="settings-row__control flex items-center justify-between gap-3">
            <span class="text-sm muted">{{ maskedEmailText }}</span>
            <NButton text type="primary" data-role="settings-bind-email" @click="openEmailModalDialog()">
              {{ hasBoundEmail ? "修改邮箱" : "绑定邮箱" }}
            </NButton>
          </div>
        </div>

        <div class="settings-row">
          <span class="settings-row__label">密码</span>
          <div class="settings-row__control flex items-center justify-between gap-3">
            <span class="text-sm muted">{{ maskedPasswordText }}</span>
            <NButton text type="primary" data-role="settings-bind-password" @click="openPasswordModalDialog()">
              {{ hasBoundPassword ? "修改密码" : "设置密码" }}
            </NButton>
          </div>
        </div>
      </div>

      <!-- 没有危险区：站点没有"注销账号"端点，放一个点了会失败的按钮只是骗人。 -->
    </NCard>

    <!-- 消息偏好 -->
    <NCard v-if="activeSection === 'notify'" class="studio-list-card" :bordered="false">
      <div class="studio-toolbar">
        <div>
          <div class="eyebrow">Preference</div>
          <h2 class="section-title mt-2">消息偏好</h2>
        </div>
        <NButton type="primary" data-role="settings-save-notify" @click="savePreference()">保存偏好</NButton>
      </div>

      <div class="mt-4">
        <div class="settings-row settings-row--switch">
          <div>
            <span class="settings-row__label">点赞提醒</span>
            <p class="settings-row__hint">有人给你的文章点赞时收到站内提醒。</p>
          </div>
          <div class="settings-row__control flex justify-end">
            <NSwitch v-model:value="preferenceForm.digg_notice_enabled" />
          </div>
        </div>

        <div class="settings-row settings-row--switch">
          <div>
            <span class="settings-row__label">评论提醒</span>
            <p class="settings-row__hint">有新的评论时收到站内提醒。</p>
          </div>
          <div class="settings-row__control flex justify-end">
            <NSwitch v-model:value="preferenceForm.comment_notice_enabled" />
          </div>
        </div>

        <div class="settings-row settings-row--switch">
          <div>
            <span class="settings-row__label">收藏提醒</span>
            <p class="settings-row__hint">有人收藏你的文章时收到站内提醒。</p>
          </div>
          <div class="settings-row__control flex justify-end">
            <NSwitch v-model:value="preferenceForm.favor_notice_enabled" />
          </div>
        </div>

        <div class="settings-row settings-row--switch">
          <div>
            <span class="settings-row__label">私信提醒</span>
            <p class="settings-row__hint">收到新私信时收到站内提醒。</p>
          </div>
          <div class="settings-row__control flex justify-end">
            <NSwitch v-model:value="preferenceForm.private_chat_notice_enabled" />
          </div>
        </div>
      </div>
    </NCard>

    <!-- 可见性：独立保存按钮 -->
    <NCard v-if="activeSection === 'visibility'" class="studio-list-card" :bordered="false">
      <div class="studio-toolbar">
        <div>
          <div class="eyebrow">Visibility</div>
          <h2 class="section-title mt-2">可见性</h2>
        </div>
        <NButton type="primary" data-role="settings-save-visibility" @click="saveVisibility()">保存可见性</NButton>
      </div>

      <div class="mt-4">
        <div class="settings-row settings-row--switch">
          <div>
            <span class="settings-row__label">收藏夹可见</span>
            <p class="settings-row__hint">关掉后，别人在你的主页看不到收藏夹。</p>
          </div>
          <div class="settings-row__control flex justify-end">
            <NSwitch v-model:value="profileForm.favorites_visibility" />
          </div>
        </div>

        <div class="settings-row settings-row--switch">
          <div>
            <span class="settings-row__label">关注列表可见</span>
            <p class="settings-row__hint">关掉后，别人看不到你关注了谁。</p>
          </div>
          <div class="settings-row__control flex justify-end">
            <NSwitch v-model:value="profileForm.followers_visibility" />
          </div>
        </div>

        <div class="settings-row settings-row--switch">
          <div>
            <span class="settings-row__label">粉丝列表可见</span>
            <p class="settings-row__hint">关掉后，别人看不到谁关注了你。</p>
          </div>
          <div class="settings-row__control flex justify-end">
            <NSwitch v-model:value="profileForm.fans_visibility" />
          </div>
        </div>
      </div>
    </NCard>

    <!-- 隐私 -->
    <NCard v-if="activeSection === 'privacy'" class="studio-list-card" :bordered="false">
      <div class="studio-toolbar">
        <div>
          <div class="eyebrow">Privacy</div>
          <h2 class="section-title mt-2">隐私</h2>
        </div>
        <NButton type="primary" data-role="settings-save-privacy" @click="savePrivacy()">保存隐私设置</NButton>
      </div>

      <div class="mt-4">
        <div class="settings-row settings-row--switch">
          <div>
            <span class="settings-row__label">允许陌生人给我发私信</span>
            <p class="settings-row__hint">
              关掉后，未关注你、也没有会话往来的人将无法给你发私信；站点总开关仍然生效。
            </p>
          </div>
          <div class="settings-row__control flex justify-end">
            <NSwitch v-model:value="profileForm.stranger_msg_enabled" />
          </div>
        </div>
      </div>
    </NCard>

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
        <ImageCaptchaField
          v-if="captchaEnabled"
          :image="captchaImage"
          :code="captchaCode"
          :loading="captchaLoading"
          @update:code="value => (captchaCode = value)"
          @refresh="refreshCaptcha"
        />
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
      v-model:show="showPhoneModal"
      preset="card"
      title="绑定手机号"
      class="w-[min(92vw,520px)]"
      :mask-closable="false">
      <div class="space-y-4">
        <label class="space-y-2 block">
          <span class="text-sm font-medium">手机号</span>
          <NInput v-model:value="phoneBindForm.phone" maxlength="11" placeholder="11 位手机号" />
        </label>
        <div class="flex flex-wrap items-center gap-3">
          <NButton secondary :disabled="phoneBindSmsLocked" @click="sendPhoneBindCode()">
            {{ phoneBindSmsLabel }}
          </NButton>
          <span class="text-xs muted">验证码发到上面这个号码，用来确认它确实在你手上</span>
        </div>
        <ImageCaptchaField
          v-if="captchaEnabled"
          :image="captchaImage"
          :code="captchaCode"
          :loading="captchaLoading"
          @update:code="value => (captchaCode = value)"
          @refresh="refreshCaptcha"
        />
        <label class="space-y-2 block">
          <span class="text-sm font-medium">短信验证码</span>
          <NInput v-model:value="phoneBindForm.code" maxlength="6" placeholder="6 位数字" />
        </label>
        <div class="flex justify-end gap-2 pt-1">
          <NButton quaternary @click="showPhoneModal = false">取消</NButton>
          <NButton type="primary" @click="submitBindPhone()">确认绑定</NButton>
        </div>
      </div>
    </NModal>

    <NModal
      v-model:show="showPasswordModal"
      preset="card"
      :title="hasBoundPassword ? '修改密码' : '设置密码'"
      class="w-[min(92vw,520px)]"
      :mask-closable="false">
      <div class="space-y-4">
        <p v-if="hasBoundPassword" class="text-sm leading-6 muted">修改密码需要先提交旧密码和新密码。</p>
        <p v-else class="text-sm leading-6 muted">
          手机号注册的账号还没有密码。设置后即可用「用户名 + 密码」登录；为防止会话被盗后被人锁死账号，这里要用绑定手机号的短信验证码确认是本人。
        </p>

        <template v-if="hasBoundPassword">
          <label class="space-y-2 block">
            <span class="text-sm font-medium">旧密码</span>
            <NInput v-model:value="passwordForm.old_password" type="password" show-password-on="click" />
          </label>
        </template>

        <template v-else>
          <label class="space-y-2 block">
            <span class="text-sm font-medium">手机号</span>
            <NInput v-model:value="passwordForm.phone" readonly placeholder="当前账号绑定的手机号" />
          </label>
          <label class="space-y-2 block">
            <span class="text-sm font-medium">短信验证码</span>
            <div class="flex gap-2">
              <NInput v-model:value="passwordForm.sms_code" maxlength="8" placeholder="输入短信验证码" />
              <NButton secondary :disabled="passwordSmsLocked || !hasBoundPhone" @click="sendPasswordCode()">
                {{ passwordSmsLabel }}
              </NButton>
            </div>
          </label>
          <ImageCaptchaField
            v-if="captchaEnabled"
            :image="captchaImage"
            :code="captchaCode"
            :loading="captchaLoading"
            @update:code="value => (captchaCode = value)"
            @refresh="refreshCaptcha"
          />
          <p v-if="!hasBoundPhone" class="text-xs muted">当前账号没有绑定手机号，暂时无法设置密码。</p>
        </template>

        <label class="space-y-2 block">
          <span class="text-sm font-medium">新密码</span>
          <NInput v-model:value="passwordForm.new_password" type="password" show-password-on="click" />
        </label>
        <div class="flex justify-end gap-2 pt-1">
          <NButton quaternary @click="showPasswordModal = false">取消</NButton>
          <NButton type="primary" @click="submitPasswordReset()">{{ hasBoundPassword ? "确认修改" : "确认设置" }}</NButton>
        </div>
      </div>
    </NModal>

    <ImageCropDialog
      :show="avatarCropRequest !== null"
      :src="avatarCropPreviewUrl"
      :image-width="avatarCropRequest?.width ?? 0"
      :image-height="avatarCropRequest?.height ?? 0"
      :target="AVATAR_SHAPE_TARGET"
      :ratio-text="AVATAR_RATIO_TEXT"
      title="裁剪头像（固定 1:1）"
      @confirm="confirmAvatarCrop"
      @cancel="cancelAvatarCrop" />
  </div>
</template>
