<script setup lang="ts">
import { ref } from "vue";
import { NButton, NCard, NInput, NModal, NSelect, NSwitch } from "naive-ui";

// 发布弹窗：补全分类/标签/封面/摘要/可见性/评论开关，然后提交。
//
// 从 pages/studio/write.vue 拆出来（M6 ⑧）。这个弹窗有二十来个绑定，
// 但全部是"显示一个值 / 上抛一个动作"，没有自己的业务判断 ——
// 所以它适合做纯展示组件，判断逻辑（存草稿还是发布、要不要审核）留在 useStudioPublish。
//
// 表单字段用 defineModel：父页面把 form 的对应字段双向绑定进来，
// 组件不直接持有 form 对象（避免"子组件偷偷改了父状态"这类问题）。

const show = defineModel<boolean>("show", { required: true });
const categoryId = defineModel<string | null>("categoryId", { required: true });
const tagIds = defineModel<string[]>("tagIds", { required: true });
const abstractText = defineModel<string>("abstractText", { required: true });
const commentsToggle = defineModel<boolean>("commentsToggle", { required: true });
const visibility = defineModel<"visible" | "user_hidden">("visibility", { required: true });

const props = defineProps<{
  /** 提交中（任一流程），此时不允许点遮罩关闭。 */
  locked: boolean;
  draftPending: boolean;
  publishPending: boolean;
  aiPending: boolean;
  /** 封面上传中：提交按钮要禁用，否则会提交到没有封面的状态。 */
  coverUploading: boolean;
  coverPreviewUrl: string;
  coverStage: string;
  categoryOptions: { label: string; value: string }[];
  tagOptions: { label: string; value: string }[];
  visibilityOptions: { label: string; value: string }[];
  draftLabel: string;
  publishLabel: string;
  /** 封面上传（原生 change 事件原样交给父级，逻辑不在这里）。 */
  onCoverChange: (event: Event) => void;
  /** 移除已选封面。 */
  onClear: () => void;
}>();

const emit = defineEmits<{
  submit: [status: 1 | 2];
  aiFill: [];
}>();

const fileInput = ref<HTMLInputElement | null>(null);

// pickCover 替父级点开文件选择：input 在本组件内，
// 让父级持有一个 ref 再回来点它没有意义。
function pickCover() {
  if (props.coverUploading) {
    return;
  }
  fileInput.value?.click();
}
</script>

<template>
  <NModal v-model:show="show" :mask-closable="!props.locked">
    <div class="publish-modal-shell">
      <NCard title="发布文章" :bordered="false" closable class="publish-modal-card" @close="show = false">
        <div class="publish-form">
          <div class="publish-form__row">
            <label class="publish-form__label">分类</label>
            <NSelect
v-model:value="categoryId" clearable filterable :options="props.categoryOptions || []"
              placeholder="请选择分类" class="publish-form__control" />
          </div>

          <div class="publish-form__row">
            <label class="publish-form__label">添加标签</label>
            <NSelect
v-model:value="tagIds" multiple clearable filterable max-tag-count="responsive"
              :options="props.tagOptions || []" placeholder="请选择已有标签" class="publish-form__control" />
          </div>

          <div class="publish-form__row publish-form__row--top">
            <label class="publish-form__label">文章封面</label>
            <div class="publish-cover">
              <input
                ref="fileInput"
                type="file"
                accept="image/*"
                class="publish-cover__file-input"
                @change="props.onCoverChange" >

              <button
                type="button"
                class="publish-cover__picker"
                :class="{ 'publish-cover__picker--busy': props.coverUploading }"
                @click="pickCover">
                <img
                  v-if="props.coverPreviewUrl"
                  :src="props.coverPreviewUrl"
                  alt="文章封面预览"
                  class="publish-cover__preview-image" >
                <template v-else>
                  <span class="publish-cover__plus">+</span>
                  <span>上传封面</span>
                </template>
              </button>

              <div class="publish-cover__actions">
                <NButton quaternary size="small" :loading="props.coverUploading" @click="pickCover">
                  {{ props.coverPreviewUrl ? "重新上传" : "选择图片" }}
                </NButton>
                <NButton v-if="props.coverPreviewUrl" quaternary size="small" @click="props.onClear">
                  移除封面
                </NButton>
              </div>
              <p v-if="props.coverStage" class="publish-cover__stage">{{ props.coverStage }}</p>
            </div>
          </div>

          <div class="publish-form__row publish-form__row--top">
            <label class="publish-form__label">编辑摘要</label>
            <div class="publish-summary">
              <NInput
v-model:value="abstractText" type="textarea" maxlength="180"
                :autosize="{ minRows: 5, maxRows: 8 }" placeholder="请输入文章摘要" />
              <span class="publish-summary__count">{{ abstractText.length }}/180</span>
            </div>
          </div>

          <div class="publish-form__row">
            <label class="publish-form__label">可见范围</label>
            <NSelect
              v-model:value="visibility"
              :options="props.visibilityOptions"
              placeholder="请选择文章可见范围"
              class="publish-form__control" />
          </div>

          <div class="publish-form__row">
            <label class="publish-form__label">评论开关</label>
            <div class="publish-form__switch">
              <span class="muted">{{ commentsToggle ? "文章发布后允许评论" : "文章发布后将关闭评论区" }}</span>
              <NSwitch v-model:value="commentsToggle" />
            </div>
          </div>

        </div>

        <template #footer>
          <div class="publish-modal__footer">
            <NButton quaternary @click="show = false">取消</NButton>
            <NButton secondary :loading="props.aiPending" @click="emit('aiFill')">AI 填入</NButton>
            <NButton quaternary :loading="props.draftPending" :disabled="props.coverUploading" @click="emit('submit', 1)">
              {{ props.draftLabel }}
            </NButton>
            <NButton type="primary" :loading="props.publishPending" :disabled="props.coverUploading" @click="emit('submit', 2)">
              {{ props.publishLabel }}
            </NButton>
          </div>
        </template>
      </NCard>
    </div>
  </NModal>
</template>
