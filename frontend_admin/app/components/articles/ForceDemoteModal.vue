<script setup lang="ts">
/**
 * 「强制退回草稿」确认框（票 票 481）。
 *
 * 为什么要有这个组件而不是在页面里各写一遍：后台有两处入口（文章列表的行操作、
 * 审核任务的处置），两处的语义必须完全一致 —— 原因必填、二次确认、成功文案、
 * 失败提示。写成两份就会漂移，而"退回草稿"是一个会通知作者的动作，两边说法不一致
 * 是最难被发现的坑。
 *
 * 为什么要填原因：它会原样出现在作者的站内信里，也是这次处置唯一的审计依据。
 * 允许留空等于允许一次没有任何理由的下架，所以这里按必填处理，按钮在空原因时禁用。
 *
 * 刻意**不**在理由里放"质量未达标"这类预置选项：预置项会让人下意识点第一个，
 * 而这条动作的使用场景恰恰是"出现了意料之外的情况"（线上漏放、误审、投诉）。
 */
import { NButton, NCard, NInput, NModal, NSpace, useMessage } from 'naive-ui'
import { forceDemoteArticle } from '~/services/admin'

const props = defineProps<{
  show: boolean
  articleId: string
  articleTitle: string
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  'done': []
}>()

const message = useMessage()
const reason = ref('')
const submitting = ref(false)

const canSubmit = computed(() => reason.value.trim().length > 0 && !submitting.value)

watch(
  () => props.show,
  (open) => {
    // 每次打开都清空：上一次的原因不该被顺手用在下一篇稿子上。
    if (open) reason.value = ''
  },
)

async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  try {
    await forceDemoteArticle(props.articleId, { reason: reason.value.trim() })
    message.success('已退回草稿，并已通知作者')
    emit('update:show', false)
    emit('done')
  } catch (error) {
    // 失败必须原样给出后端的业务文案：403 / 参数错误 / 文章已删除是不同处置，
    // 统一显示"操作失败"会让运营反复点同一个按钮。
    message.error(error instanceof Error ? error.message : '退回失败')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    class="admin-force-demote-modal"
    title="强制退回草稿"
    @update:show="emit('update:show', $event)"
  >
    <div class="admin-settings-panel">
      <NCard size="small" class="admin-card mb-3">
        <p class="m-0 text-sm">
          将把「{{ articleTitle }}」从已发布状态<strong>强制退回草稿</strong>：
          读者立刻看不到它，作者会收到一条带原因的站内信，这次处置也会写进审核记录。
        </p>
      </NCard>

      <NInput
        v-model:value="reason"
        type="textarea"
        :autosize="{ minRows: 3, maxRows: 6 }"
        :maxlength="200"
        show-count
        placeholder="填写退回原因（必填）：它会原样出现在作者收到的站内信里"
        data-role="force-demote-reason"
      />
    </div>

    <template #footer>
      <NSpace justify="end">
        <NButton @click="emit('update:show', false)">取消</NButton>
        <NButton type="error" :disabled="!canSubmit" :loading="submitting" @click="submit">
          确认退回草稿
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>
