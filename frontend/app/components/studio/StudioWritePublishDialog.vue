<script setup lang="ts">
import { computed, ref } from "vue";
import { NAlert, NButton, NCard, NInput, NModal, NSelect, NSwitch } from "naive-ui";

import StudioCoverUploadPanel from "./StudioCoverUploadPanel.vue";
import type { QualityRejectionDetails } from "~/composables/useStudioPublish";
import type { ShapeImageTarget } from "~/utils/imageShaping";
import { categoryTitleError, normalizeCategoryTitle } from "~/utils/studioCategory";

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
  /** 封面预览（本地定形产物优先，上传成功后是线上地址）。 */
  coverPreviewUrl: string;
  /** 上传阶段 / 留在面板里的错误文案。 */
  coverStage: string;
  coverError: string;
  /** 选过图之后才有定形产物尺寸（回填的旧封面没有）。 */
  coverSize: ShapeImageTarget | null;
  categoryOptions: { label: string; value: string }[];
  /**
   * 分类下拉加载失败的可读原因（票 320）。
   *
   * 空串表示这次加载没出错 —— 与"确实一个分类都没有"必须分开说，
   * 否则用户会以为账号里没有分类，转头去重复建同名。
   */
  categoryError?: string;
  tagOptions: { label: string; value: string }[];
  /**
   * 标签下拉加载失败的可读原因。
   *
   * 与 `categoryError` 同一套规矩（票 320 的病根在标签侧也存在过）：
   * 空串表示这次加载没出错，失败时必须说清是"没加载出来"，而不是空下拉。
   */
  tagError?: string;
  visibilityOptions: { label: string; value: string }[];
  draftLabel: string;
  publishLabel: string;
  /**
   * 最近一次「质量不达标」的完整结论（票 306）。
   *
   * null 表示这次提交没被闸门拦下；有值时在弹窗里就地展示分数、差额与
   * 问题列表——弹窗本来就没关（提交失败不会关），作者不用再去找提示。
   */
  qualityRejection?: QualityRejectionDetails | null;
  /**
   * 当前文章是否仍在"已发布、等内容检查通过才对读者可见"（票 302）。
   *
   * 只影响一句提示，不改变按钮行为：作者仍可编辑重投，服务端会重新判定。
   */
  pendingReview?: boolean;
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

              <!-- 分类下拉的两种"空"（票 320）：没数据 vs 没加载出来。
                   前者给一句能照着做的话，后者给原因 —— 都不能是一声不响的空白。 -->
              <p
                v-if="props.categoryError"
                class="publish-category__hint publish-category__hint--error"
                data-role="category-error">
                {{ props.categoryError }}
              </p>
              <p v-else-if="!(props.categoryOptions || []).length" class="publish-category__hint" data-role="category-empty">
                {{ props.canCreateCategory ? "还没有分类，点下面「+ 新建分类」创建第一个" : "还没有可选分类" }}
              </p>

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
            <div class="publish-tags">
              <NSelect
v-model:value="tagIds" multiple clearable filterable max-tag-count="responsive"
                :options="props.tagOptions || []" placeholder="请选择已有标签" class="publish-form__control" />

              <!-- 标签下拉的故障态（与分类同款）：接口挂了不能装成"还没有标签"。 -->
              <p v-if="props.tagError" class="publish-tags__hint publish-tags__hint--error" data-role="tag-error">
                {{ props.tagError }}
              </p>
            </div>
          </div>

          <div class="publish-form__row publish-form__row--top">
            <label class="publish-form__label">文章封面</label>
            <StudioCoverUploadPanel
              :busy="props.coverUploading"
              :preview-url="props.coverPreviewUrl"
              :stage="props.coverStage"
              :error="props.coverError"
              :size="props.coverSize"
              :on-file-change="props.onCoverChange"
              :on-clear="props.onClear" />
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

          <!-- 质量不达标（票 306）：就地展示分数、差额与逐条问题。
               弹窗在失败后仍然是打开的，所以这里比 toast 更合适：作者能一条条
               对照着改，而不是在一句会消失的提示里找信息。 -->
          <NAlert
            v-if="props.qualityRejection"
            type="error"
            :bordered="false"
            class="publish-form__rejection"
            data-role="quality-rejection">
            <template #header>
              内容质量未达标：{{ props.qualityRejection.score }} 分，
              放行线 {{ props.qualityRejection.required_score }} 分，
              还差 {{ props.qualityRejection.gap }} 分
            </template>
            <ul v-if="props.qualityRejection.issues?.length" class="publish-rejection__list">
              <li v-for="(issue, index) in props.qualityRejection.issues" :key="index">
                <strong>{{ issue.reason || "需要修改" }}</strong>
                <span v-if="issue.suggestion"> —— {{ issue.suggestion }}</span>
              </li>
            </ul>
            <p v-else class="muted m-0">这篇文章已落回草稿，改完可以立刻重投。</p>
          </NAlert>

          <!-- 票 302：文章已发布但还在等内容检查，读者暂时看不到。
               只给一句状态说明，不暴露命中词、模型结论或内部状态名。 -->
          <NAlert
            v-if="props.pendingReview && !props.qualityRejection"
            type="info"
            :bordered="false"
            class="publish-form__rejection"
            data-role="pending-review">
            文章已发布，正在做内容检查；检查通过后才会对读者可见。期间仍可继续编辑并重新提交。
          </NAlert>

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

<style scoped>
/*
 * 弹窗自己的尺寸约束必须写在**本组件**里。
 *
 * 这些规则原来放在 pages/studio/write.vue 的 `<style scoped src="...studio-write.css">` 里：
 * 页面的 scoped 样式只作用于该页模板，而 NModal 把内容 teleport 到 body，
 * 弹窗内部的元素拿不到页面的 data-v 作用域，于是宽度规则整条失效 ——
 * NCard 按默认宽度铺满视口，发布信息看起来就是"全屏卡片"而不是弹窗。
 */
.publish-modal-shell {
  width: min(760px, calc(100vw - 96px));
  max-height: calc(100vh - 96px);
  margin: 0 auto;
  overflow: auto;
}

.publish-modal-card {
  border-radius: var(--radius-xl);
}

.publish-form {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.publish-form__row {
  display: grid;
  grid-template-columns: 84px minmax(0, 1fr);
  align-items: center;
  gap: 18px;
}

.publish-form__row--top {
  align-items: flex-start;
}

.publish-form__label {
  font-size: 14px;
  color: var(--color-ink);
}

.publish-form__control {
  width: 100%;
}

/* 分类下拉 + 内联新建入口。新建入口与下拉同列，展开时不推挤其它行。 */
.publish-category {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 8px;
}

/* 标签下拉 + 它的故障提示：与分类同列布局（提示挂在控件下面）。 */
.publish-tags {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 8px;
}

.publish-category__create {
  display: flex;
  align-items: center;
  gap: 8px;
}

.publish-category__input {
  flex: 1;
  min-width: 0;
}

.publish-category__add {
  align-self: flex-start;
}

/* 分类下拉的空状态（正常态）与错误（故障态）：一句话之差，颜色与语气都要区分。
   标签下拉的错误提示用同一套样式（.publish-tags__hint），不另立一套口径。 */
.publish-category__hint,
.publish-tags__hint {
  margin: 0;
  color: var(--color-ink-muted);
  font-size: 12px;
  line-height: 1.5;
}

.publish-category__hint--error,
.publish-tags__hint--error {
  color: var(--color-danger-ink);
}

.publish-summary {
  position: relative;
}

.publish-summary__count {
  position: absolute;
  right: 12px;
  bottom: 10px;
  color: var(--color-danger-ink);
  font-size: 12px;
}

.publish-form__switch {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface-raised);
}

.publish-modal__footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

@media (max-width: 767px) {
  .publish-modal-shell {
    width: calc(100vw - 28px);
    max-height: calc(100vh - 28px);
  }

  .publish-form__row {
    grid-template-columns: 1fr;
    gap: 10px;
  }

  .publish-modal__footer {
    flex-wrap: wrap;
  }
}
</style>
