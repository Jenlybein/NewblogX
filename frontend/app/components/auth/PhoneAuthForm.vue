<script setup lang="ts">
import { toTypedSchema } from "@vee-validate/zod";
import { useForm } from "vee-validate";
import { z } from "zod";
import { NButton, NForm, NFormItem, NInput, useMessage } from "naive-ui";
import { sendSmsCode } from "~/services/auth";
import { errorMessage } from "~/utils/errorMessage";
import { useVerificationCountdown } from "~/composables/useVerificationCountdown";
import { useCaptchaProof } from "~/composables/useCaptchaProof";
import ImageCaptchaField from "~/components/auth/ImageCaptchaField.vue";

const emit = defineEmits<{
  success: [];
}>();

const authStore = useAuthStore();
const message = useMessage();
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

/** 与后端 domain.ValidatePhone 同一形状：11 位、1 开头、第二位 3-9。 */
const phonePattern = /^1[3-9]\d{9}$/;

const schema = toTypedSchema(
  z.object({
    phone: z.string().regex(phonePattern, "请输入 11 位手机号"),
    code: z.string().min(4, "请输入验证码"),
  }),
);

const { defineField, handleSubmit, errors, values } = useForm({
  validationSchema: schema,
  initialValues: {
    phone: "",
    code: "",
  },
});

const [phone, phoneProps] = defineField("phone");
const [code, codeProps] = defineField("code");

async function handleSendCode() {
  const phoneValue = (values.phone ?? "").trim();
  if (!phonePattern.test(phoneValue)) {
    message.warning("请先输入 11 位手机号");
    return;
  }

  if (captchaMissing.value) {
    message.warning("请输入图形验证码");
    return;
  }

  try {
    await sendSmsCode({ phone: phoneValue, purpose: "login", captcha: captchaPayload() });
    startCountdown();
    // 图形码是一次性的：校验过就作废，换一张给下一次发码用。
    void refreshCaptcha();
    message.success("验证码已发送");
  } catch (error) {
    message.error(errorMessage(error, "发送失败"));
  }
}

const onSubmit = handleSubmit(async (formValues) => {
  try {
    await authStore.loginByPhone({ phone: formValues.phone, code: formValues.code });
    message.success("登录成功");
    emit("success");
  } catch (error) {
    message.error(errorMessage(error, "登录失败"));
  }
});
</script>

<template>
  <NForm @submit.prevent="onSubmit">
    <NFormItem label="手机号" :feedback="errors.phone">
      <NInput v-model:value="phone" v-bind="phoneProps" maxlength="11" placeholder="输入 11 位手机号" />
    </NFormItem>
    <NFormItem label="验证码" :feedback="errors.code">
      <div class="flex w-full gap-3">
        <NInput v-model:value="code" v-bind="codeProps" maxlength="8" placeholder="输入短信验证码" />
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
    <NButton type="primary" attr-type="submit" block :loading="authStore.pending">
      登录 / 注册
    </NButton>
    <p class="mt-3 text-center text-xs muted">未注册的手机号验证通过后将自动创建账号</p>
  </NForm>
</template>
