<script setup lang="ts">
import { ref } from "vue";
import { toTypedSchema } from "@vee-validate/zod";
import { useForm } from "vee-validate";
import { z } from "zod";
import { NButton, NForm, NFormItem, NInput, useMessage } from "naive-ui";
import { sendEmailCode } from "~/services/auth";
import { errorMessage } from "~/utils/errorMessage";
import { useVerificationCountdown } from "~/composables/useVerificationCountdown";
import { useCaptchaProof } from "~/composables/useCaptchaProof";
import ImageCaptchaField from "~/components/auth/ImageCaptchaField.vue";

const emit = defineEmits<{
  success: [];
}>();

const authStore = useAuthStore();
const message = useMessage();
const emailId = ref("");
const { counting: sendDisabled, sendLabel, start: startCountdown } = useVerificationCountdown();
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
    email: z.string().email("请输入有效邮箱"),
    email_code: z.string().min(4, "请输入验证码"),
    pwd: z.string().min(6, "密码至少 6 位"),
  }),
);

const { defineField, handleSubmit, errors, values } = useForm({
  validationSchema: schema,
  initialValues: {
    email: "",
    email_code: "",
    pwd: "",
  },
});

const [email, emailProps] = defineField("email");
const [emailCode, emailCodeProps] = defineField("email_code");
const [password, passwordProps] = defineField("pwd");

async function handleSendCode() {
  if (!values.email) {
    message.warning("请先输入邮箱");
    return;
  }

  if (captchaMissing.value) {
    message.warning("请输入图形验证码");
    return;
  }

  try {
    const payload = await sendEmailCode({
      email: values.email,
      purpose: "register",
      captcha: captchaPayload(),
    });
    emailId.value = payload.id;
    startCountdown();
    // 图形码是一次性的：校验过就作废，换一张给下一次发码用。
    void refreshCaptcha();
    message.success("验证码已发送");
  } catch (error) {
    message.error(errorMessage(error, "发送失败"));
    // 失败也换一张：图形码一次性且会过期，沿用旧图重试基本等于白试。
    void refreshCaptcha();
  }
}

const onSubmit = handleSubmit(async (formValues) => {
  if (!emailId.value) {
    message.warning("请先发送验证码");
    return;
  }

  try {
    await authStore.registerByEmail({
      pwd: formValues.pwd,
      email_id: emailId.value,
      email_code: formValues.email_code,
    });
    message.success("注册并登录成功");
    emit("success");
  } catch (error) {
    message.error(errorMessage(error, "注册失败"));
  }
});
</script>

<template>
  <NForm @submit.prevent="onSubmit">
    <NFormItem label="邮箱" :feedback="errors.email">
      <NInput v-model:value="email" v-bind="emailProps" placeholder="输入邮箱" />
    </NFormItem>
    <NFormItem label="验证码" :feedback="errors.email_code">
      <div class="flex w-full gap-3">
        <NInput v-model:value="emailCode" v-bind="emailCodeProps" placeholder="输入邮箱验证码" />
        <NButton secondary :disabled="sendDisabled" @click="handleSendCode">
          {{ sendLabel }}
        </NButton>
      </div>
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
    <NFormItem label="密码" :feedback="errors.pwd">
      <NInput
        v-model:value="password"
        v-bind="passwordProps"
        type="password"
        show-password-on="click"
        placeholder="设置密码"
      />
    </NFormItem>
    <NButton type="primary" attr-type="submit" block :loading="authStore.pending">
      邮箱注册
    </NButton>
  </NForm>
</template>
