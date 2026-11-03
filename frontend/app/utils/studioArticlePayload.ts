// 文章提交载荷的组装规则。
//
// 从 composables/useStudioArticle 里提出来（M6 ⑧）。提出来有两个原因：
//   1. 它是**纯函数**（输入表单、输出载荷），不该和 route / store / useAsyncData
//      绑在一个模块里 —— 绑在一起的结果就是"想测它必须先 mock 整个 Nuxt 上下文"；
//   2. 这里的规则看着琐碎，但每条都会直接改线上数据的语义（见下面注释）。
//
// 放在 utils 而不是 composables：它不持有任何响应式状态。

/** 提交载荷的表单部分。 */
export type StudioArticleForm = {
  title: string;
  abstract: string;
  content: string;
  category_id: string | null;
  tag_ids: string[];
  comments_toggle: boolean;
  visibility_status: "visible" | "user_hidden";
  cover_image_id: string | null;
};

/** 提交载荷。字段可选性由后端约定决定（见 services/article）。 */
export type StudioArticlePayload = {
  title: string;
  abstract?: string;
  content: string;
  category_id: string | null;
  tag_ids: string[];
  comments_toggle: boolean;
  visibility_status: "visible" | "user_hidden";
  status: 1 | 2;
  cover_image_id?: string | null;
};

/**
 * buildStudioArticlePayload 把表单拼成提交载荷。
 *
 * 提成模块级纯函数（不依赖闭包）是为了能单测：这里的规则看着琐碎但会直接影响
 * 线上数据的三个地方 ——
 *
 *  1. `abstract` 空串要变成 `undefined`（不是 `""`）：后端对 `""` 的处理是
 *     "写一个空摘要"，对 `undefined` 是"不改"，语义完全不同；
 *  2. `category_id` 空串同样要变 `null`；
 *  3. `cover_image_id` 有三种情况：有值就带上；编辑态且用户主动清过封面时
 *     要显式传 `null`（表示"删掉封面"）；其余情况不传（表示"不动"）。
 *     漏掉第三种会让"移除封面"静默失效。
 */
export function buildStudioArticlePayload(
  form: StudioArticleForm,
  opts: { status: 1 | 2; isEditMode: boolean; coverDirty: boolean },
): StudioArticlePayload {
  const payload: StudioArticlePayload = {
    title: form.title.trim(),
    abstract: form.abstract.trim() || undefined,
    content: form.content,
    category_id: form.category_id || null,
    tag_ids: form.tag_ids,
    comments_toggle: form.comments_toggle,
    visibility_status: form.visibility_status,
    status: opts.status,
  };

  if (form.cover_image_id) {
    payload.cover_image_id = form.cover_image_id;
  } else if (opts.isEditMode && opts.coverDirty) {
    payload.cover_image_id = null;
  }

  return payload;
}

