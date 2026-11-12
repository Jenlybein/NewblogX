<script setup lang="ts">
import { toTypedSchema } from "@vee-validate/zod";
import { useForm } from "vee-validate";
import { z } from "zod";
import { NButton, NForm, NFormItem, NInput, useMessage } from "naive-ui";
import { errorMessage } from "~/utils/errorMessage";
import { useCaptchaProof } from "~/composables/useCaptchaProof";
import ImageCaptchaField from "~/components/auth/ImageCaptchaField.vue";

const emit = defineEmits<{
  success: [];
}>();

const authStore = useAuthStore();
const message = useMessage();
const {
  enabled: captchaEnabled,
  image: captchaImage,
  code: captchaCode,
  loading: captchaLoading,
  missing: captchaMissing,
  refresh: refreshCaptcha,
  payload: captchaPayload,
} = useCaptchaProof();

const schema = toTypedSchema(
  z.object({
    username: z.string().min(3, "请输入用户名"),
    password: z.string().min(6, "密码至少 6 位"),
  }),
);

const { defineField, handleSubmit, errors, isSubmitting } = useForm({
  validationSchema: schema,
  initialValues: {
    username: "",
    password: "",
  },
});

const [username, usernameProps] = defineField("username");
const [password, passwordProps] = defineField("password");

const onSubmit = handleSubmit(async (values) => {
  if (captchaMissing.value) {
    message.warning("请输入图形验证码");
    return;
  }

  try {
    await authStore.loginByPassword({ ...values, captcha: captchaPayload() });
    message.success("登录成功");
    emit("success");
  } catch (error) {
    message.error(errorMessage(error, "登录失败"));
    // 失败也换一张：图形码一次性，重试时沿用旧图多半是白试。
    void refreshCaptcha();
  }
});
</script>

<template>
  <NForm @submit.prevent="onSubmit">
    <NFormItem label="用户名" :feedback="errors.username">
      <NInput v-model:value="username" v-bind="usernameProps" placeholder="输入用户名" />
    </NFormItem>
    <NFormItem label="密码" :feedback="errors.password">
      <NInput
        v-model:value="password"
        v-bind="passwordProps"
        type="password"
        show-password-on="click"
        placeholder="输入密码"
      />
    </NFormItem>
    <NFormItem v-if="captchaEnabled">
      <ImageCaptchaField
        :image="captchaImage"
        :code="captchaCode"
        :loading="captchaLoading"
        @update:code="value => (captchaCode = value)"
        @refresh="refreshCaptcha"
      />
    </NFormItem>
    <NButton type="primary" attr-type="submit" block :loading="isSubmitting || authStore.pending">
      登录
    </NButton>
  </NForm>
</template>
