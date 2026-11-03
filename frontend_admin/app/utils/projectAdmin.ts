import type { ProjectCreatePayload, ProjectUpdatePayload } from '~/types/api'
import { applyDisplayResult, resolveCollectionCover } from '~/utils/collectionAdmin'
import type { CollectionCoverView } from '~/utils/collectionAdmin'

/**
 * 后台项目管理页的编排算术（与合集管理页镜像）。
 *
 * 页面里真正会出事的四处都不是渲染，而是
 *   1. 标题空 / 超长会不会被提交；
 *   2. 封面是不是在**上传前**就定形成 16:9（票 200 起，封面不再有取景参数）；
 *   3. 没上传封面时列表显示的是不是读者会看到的那张，且两张都没有时不发空 img；
 *   4. 展示开关的请求结果有没有落到列表上。
 *
 * 封面上传前的定形走 `~/utils/imageShaping` 的同一份实现（三种封面共用），
 * 本模块只负责表单与读模型的投影。封面回退与展示开关的结果落表复用合集那两
 * 个通用纯函数——两份实现一旦分叉，两个页面就会对同一行给出不同结论。
 * （不做同名 re-export：Nuxt 的 utils 自动导入会因此报"重复导入"。）
 */

/** 标题上限，与后端 `project.MaxTitleLen` 同一取值。 */
export const PROJECT_TITLE_MAX = 128

/** 描述上限，与后端 `project.MaxDescriptionLen` 同一取值。 */
export const PROJECT_DESCRIPTION_MAX = 512

/** 表单里的三个可编辑字段（封面是已上传的图片地址）。 */
export interface ProjectFormValues {
  title: string
  description: string
  cover: string
}

/** 表单校验结果；`null` 表示该字段合法。 */
export interface ProjectFormErrors {
  title: string | null
  description: string | null
}

/** 按字符数（不是字节数）量长度：中文一个字算一个。 */
function charCount(value: string): number {
  return Array.from(value).length
}

/** 校验项目表单：标题非空且不超长，描述可空但不超长。 */
export function validateProjectForm(values: ProjectFormValues): ProjectFormErrors {
  const title = values.title.trim()
  const description = values.description.trim()

  let titleError: string | null = null
  if (!title) {
    titleError = '项目标题不能为空'
  } else if (charCount(title) > PROJECT_TITLE_MAX) {
    titleError = `项目标题不能超过 ${PROJECT_TITLE_MAX} 个字符`
  }

  const descriptionError =
    charCount(description) > PROJECT_DESCRIPTION_MAX
      ? `项目描述不能超过 ${PROJECT_DESCRIPTION_MAX} 个字符`
      : null

  return { title: titleError, description: descriptionError }
}

/** 建项目的请求体：字段全部显式给，空封面也要发空串（后端据此回退首篇）。 */
export function buildProjectCreatePayload(
  values: ProjectFormValues & { displayed: boolean },
): ProjectCreatePayload {
  return {
    title: values.title.trim(),
    description: values.description.trim(),
    cover: values.cover.trim(),
    displayed: values.displayed,
  }
}

/** 改项目的请求体：只提交内容，不带展示开关（免得覆盖别人刚切的状态）。 */
export function buildProjectUpdatePayload(values: ProjectFormValues): ProjectUpdatePayload {
  return {
    title: values.title.trim(),
    description: values.description.trim(),
    cover: values.cover.trim(),
  }
}

/** 列表与表单预览要显示的封面（与合集同一份形状）。 */
export type ProjectCoverView = CollectionCoverView

/**
 * 解析项目最终要显示的封面。
 *
 * 优先用项目自己上传的封面；没有就回退到首篇公开文章的封面；两张都没有时
 * 返回 `null` —— 调用方必须据此渲染显式占位，**不能**渲染一个 `src=""` 的 img。
 */
export function resolveProjectCover(
  cover: string | null | undefined,
  fallbackCover: string | null | undefined,
): ProjectCoverView | null {
  return resolveCollectionCover(cover, fallbackCover)
}

/** 把一次展示开关的结果落到列表：只改目标行，返回新数组。 */
export function applyProjectDisplayResult<T extends { id: string; displayed: boolean }>(
  rows: T[],
  id: string,
  displayed: boolean,
): T[] {
  return applyDisplayResult(rows, id, displayed)
}
