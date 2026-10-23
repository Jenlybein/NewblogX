<script setup lang="ts">
import { toTypedSchema } from "@vee-validate/zod";
import { useForm } from "vee-validate";
import { z } from "zod";
import { NButton, NForm, NFormItem, NInput, useMessage } from "naive-ui";

const emit = defineEmits<{
  success: [];
}>();

const authStore = useAuthStore();
const message = useMessage();

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
  try {
    await authStore.loginByPassword(values);
    message.success("登录成功");
    emit("success");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "登录失败");
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
    <NButton type="primary" attr-type="submit" block :loading="isSubmitting || authStore.pending">
      登录
    </NButton>
  </NForm>
</template>
