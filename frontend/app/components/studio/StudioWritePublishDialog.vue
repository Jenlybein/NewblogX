<script setup lang="ts">
import { computed, ref } from "vue";
import type { CSSProperties } from "vue";
import { NButton, NCard, NInput, NModal, NSelect, NSwitch } from "naive-ui";

import StudioCoverUploadPanel from "./StudioCoverUploadPanel.vue";
import type { Size } from "~/utils/coverFraming";
import { categoryTitleError, normalizeCategoryTitle } from "~/utils/studioCategory";
import type { CoverUploadRoute } from "~/utils/studioCover";

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
  /** 封面预览（本地取景预览优先，上传成功后是线上地址）。 */
  coverPreviewUrl: string;
  /** 上传阶段 / 留在面板里的错误文案。 */
  coverStage: string;
  coverError: string;
  /** 选过图之后才有原图尺寸与上传路径（回填的旧封面两者都没有）。 */
  coverSize: Size | null;
  coverRoute: CoverUploadRoute | "";
  /** 取景参数是否已确认（只影响徽标与"调整取景"文案）。 */
  coverFramed: boolean;
  /** 面板预览的取景样式（与取景窗同一出口）。 */
  coverPreviewStyle: CSSProperties | null;
  categoryOptions: { label: string; value: string }[];
  tagOptions: { label: string; value: string }[];
  visibilityOptions: { label: string; value: string }[];
  draftLabel: string;
  publishLabel: string;
  /** 站点草稿开关关闭：保存草稿按钮禁用（服务端同样会拒绝）。 */
  draftDisabled: boolean;
  /** 站点发布开关关闭：发布按钮禁用（服务端同样会拒绝）。 */
  publishDisabled: boolean;
  /** 登录后分类旁才出现「新建分类」入口（未登录不可用）。 */
  canCreateCategory: boolean;
  /** 内联新建分类进行中：创建按钮转圈。 */
  categoryCreating: boolean;
  /** 新建分类（逻辑在 useStudioArticle；本组件只上抛标题并接管失败时的输入保留）。 */
  onCreateCategory: (title: string) => Promise<boolean>;
  /** 封面上传（原生 change 事件原样交给父级，逻辑不在这里）。 */
  onCoverChange: (event: Event) => void;
  /** 移除已选封面。 */
  onClear: () => void;
  /** 打开取景窗（手动取景 / 调整取景）。 */
  onFrame: () => void;
}>();

const emit = defineEmits<{
  submit: [status: 1 | 2];
  aiFill: [];
}>();

// 内联新建分类：输入框的开关与草稿标题都是纯 UI 状态，判断逻辑留在 composable。
const categoryCreateOpen = ref(false);
const newCategoryTitle = ref("");
const newCategoryTitleError = computed(() => categoryTitleError(newCategoryTitle.value));

function toggleCategoryCreate() {
  categoryCreateOpen.value = !categoryCreateOpen.value;
  if (!categoryCreateOpen.value) {
    newCategoryTitle.value = "";
  }
}

async function submitCategory() {
  if (props.categoryCreating || newCategoryTitleError.value) {
    return;
  }
  const created = await props.onCreateCategory(normalizeCategoryTitle(newCategoryTitle.value));
  // 只有真正完成"创建并选中"才收起面板；重名/网络失败时保留输入，让用户改名重试。
  if (created) {
    newCategoryTitle.value = "";
    categoryCreateOpen.value = false;
  }
}
</script>

<template>
  <NModal v-model:show="show" :mask-closable="!props.locked">
    <div class="publish-modal-shell">
      <NCard title="发布文章" :bordered="false" closable class="publish-modal-card" @close="show = false">
        <div class="publish-form">
          <div class="publish-form__row">
            <label class="publish-form__label">分类</label>
            <div class="publish-category">
              <NSelect
v-model:value="categoryId" clearable filterable :options="props.categoryOptions || []"
                placeholder="请选择分类" class="publish-form__control" />

              <!-- 新建分类就地展开：不跳分类管理页，创建成功后自动选中（issue 127）。 -->
              <template v-if="props.canCreateCategory">
                <div v-if="categoryCreateOpen" class="publish-category__create">
                  <NInput
                    v-model:value="newCategoryTitle"
                    size="small"
                    maxlength="256"
                    placeholder="新分类名称"
                    class="publish-category__input"
                    @keyup.enter="submitCategory" />
                  <NButton
                    type="primary"
                    size="small"
                    :loading="props.categoryCreating"
                    :disabled="Boolean(newCategoryTitleError)"
                    @click="submitCategory">
                    创建
                  </NButton>
                  <NButton quaternary size="small" @click="toggleCategoryCreate">取消</NButton>
                </div>
                <NButton v-else quaternary size="small" class="publish-category__add" @click="toggleCategoryCreate">
                  + 新建分类
                </NButton>
              </template>
            </div>
          </div>

          <div class="publish-form__row">
            <label class="publish-form__label">添加标签</label>
            <NSelect
v-model:value="tagIds" multiple clearable filterable max-tag-count="responsive"
              :options="props.tagOptions || []" placeholder="请选择已有标签" class="publish-form__control" />
          </div>

          <div class="publish-form__row publish-form__row--top">
            <label class="publish-form__label">文章封面</label>
            <StudioCoverUploadPanel
              :busy="props.coverUploading"
              :preview-url="props.coverPreviewUrl"
              :stage="props.coverStage"
              :error="props.coverError"
              :size="props.coverSize"
              :route="props.coverRoute"
              :framed="props.coverFramed"
              :preview-style="props.coverPreviewStyle"
              :on-file-change="props.onCoverChange"
              :on-clear="props.onClear"
              :on-frame="props.onFrame" />
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
            <NButton
              quaternary
              :loading="props.draftPending"
              :disabled="props.coverUploading || props.draftDisabled"
              :title="props.draftDisabled ? '站点当前不允许保存草稿（站点策略）' : undefined"
              @click="emit('submit', 1)">
              {{ props.draftLabel }}
            </NButton>
            <NButton
              type="primary"
              :loading="props.publishPending"
              :disabled="props.coverUploading || props.publishDisabled"
              :title="props.publishDisabled ? '站点当前不允许发布文章（站点策略）' : undefined"
              @click="emit('submit', 2)">
              {{ props.publishLabel }}
            </NButton>
          </div>
        </template>
      </NCard>
    </div>
  </NModal>
</template>
