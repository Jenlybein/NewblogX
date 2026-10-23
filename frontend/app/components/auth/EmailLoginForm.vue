<script setup lang="ts">
import { computed, ref } from "vue";
import { toTypedSchema } from "@vee-validate/zod";
import { useForm } from "vee-validate";
import { z } from "zod";
import { NButton, NForm, NFormItem, NInput, useMessage } from "naive-ui";
import { sendEmailCode } from "~/services/auth";

const emit = defineEmits<{
  success: [];
}>();

const authStore = useAuthStore();
const message = useMessage();
const emailId = ref("");
const countdown = ref(0);
let timer: ReturnType<typeof setInterval> | null = null;

const schema = toTypedSchema(
  z.object({
    email: z.string().email("请输入有效邮箱"),
    email_code: z.string().min(4, "请输入验证码"),
  }),
);

const { defineField, handleSubmit, errors, values } = useForm({
  validationSchema: schema,
  initialValues: {
    email: "",
    email_code: "",
  },
});

const [email, emailProps] = defineField("email");
const [emailCode, emailCodeProps] = defineField("email_code");

const sendDisabled = computed(() => countdown.value > 0);

function startCountdown() {
  countdown.value = 60;
  timer && clearInterval(timer);
  timer = setInterval(() => {
    countdown.value -= 1;
    if (countdown.value <= 0) {
      timer && clearInterval(timer);
      timer = null;
    }
  }, 1000);
}

async function handleSendCode() {
  if (!values.email) {
    message.warning("请先输入邮箱");
    return;
  }

  try {
    const payload = await sendEmailCode({
      email: values.email,
      type: 1,
    });
    emailId.value = payload.email_id;
    startCountdown();
    message.success("验证码已发送");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "发送失败");
  }
}

const onSubmit = handleSubmit(async (formValues) => {
  if (!emailId.value) {
    message.warning("请先发送验证码");
    return;
  }

  try {
    await authStore.loginByEmailCode({
      email_id: emailId.value,
      email_code: formValues.email_code,
    });
    message.success("登录成功");
    emit("success");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "登录失败");
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
          {{ sendDisabled ? `${countdown}s` : "发送验证码" }}
        </NButton>
      </div>
    </NFormItem>
    <NButton type="primary" attr-type="submit" block :loading="authStore.pending">
      邮箱登录
    </NButton>
  </NForm>
</template>
