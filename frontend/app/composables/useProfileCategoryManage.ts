import { computed, ref } from "vue";
import { deleteProfileCategories, saveProfileCategory } from "~/services/category";
import { errorMessage } from "~/utils/errorMessage";
import type { ProfileCategoryItem } from "~/types/api";

/**
 * 个人主页分类栏的写操作（票 325）：新建、改名、删除。
 *
 * 为什么单独成 composable：页面（`pages/users/[id].vue`）已经要管五个分栏的取数，
 * 写操作连同"二次确认 + 提示 + 重拉列表"塞进去会把它变成第二个写作页。
 * 这里只留状态与流程，UI 两件东西由调用方注入：
 *
 *  - `message`：提示能力（与 useStudioArticle 同一做法：只声明用到的方法，
 *    单测里换成记录器，不依赖 naive-ui 的 provider）；
 *  - `confirm`：二次确认。页面传 naive-ui 的 `dialog.warning`，测试传记录器。
 *
 * 三条语义跟后端对齐，缺一条用户就会看到"删了但没删干净"：
 *  1. **删除前告知受影响篇数**：分类下的文章会变成"未分类"，文章本身不删；
 *  2. **失败不假装成功**：后端给的可读文案（空名 / 名称重复 / 分类不存在）原样展示，
 *     弹窗开着、列表不重拉；
 *  3. **写完全部以服务端为准**：新建（可能恢复的是软删那一条）、改名、删除都重拉列表，
 *     前端不自己拼条目——恢复保留原 ID，本地猜会猜错。
 */

/** 本模块需要的提示能力（只声明用到的三个方法）。 */
export type CategoryManageNotifier = {
  success: (msg: string) => void;
  warning: (msg: string) => void;
  error: (msg: string) => void;
};

/** 一次二次确认的请求；`onConfirm` 是"用户点了确定"之后要跑的事情。 */
export interface CategoryManageConfirmRequest {
  title: string;
  content: string;
  onConfirm: () => Promise<void>;
}

/** 写操作作用的那一条分类（只用到分类栏渲染得出的三个字段）。 */
export type ManageableCategory = Pick<ProfileCategoryItem, "id" | "title" | "article_count">;

export interface ProfileCategoryManageOptions {
  message: CategoryManageNotifier;
  confirm: (request: CategoryManageConfirmRequest) => void;
  /** 写完重拉分类列表（以服务端为准）。 */
  reload: () => Promise<unknown> | unknown;
  /** 删除成功后通知调用方（页面据此收起正在展开的那个分类）。 */
  onDeleted?: (id: string) => void;
}

export function useProfileCategoryManage(options: ProfileCategoryManageOptions) {
  const formOpen = ref(false);
  const formTitle = ref("");
  /** 正在改名的分类；null 表示"新建"。 */
  const editingCategory = ref<ManageableCategory | null>(null);
  const saving = ref(false);

  const formMode = computed<"create" | "rename">(() => (editingCategory.value ? "rename" : "create"));

  function openCreate() {
    editingCategory.value = null;
    formTitle.value = "";
    formOpen.value = true;
  }

  function openRename(category: ManageableCategory) {
    editingCategory.value = category;
    formTitle.value = category.title;
    formOpen.value = true;
  }

  /**
   * 提交新建 / 改名。
   *
   * `title` 是弹窗 `emit('submit', title)` 带过来的用户输入；不传时回落到
   * `formTitle`（供"不经过弹窗"的调用点与既有用例使用）。
   *
   * 为什么必须有这个参数：弹窗自己持有输入框、只在 `initialTitle` 变化时回填
   * `formTitle`，所以用户敲的字**不会**写回 `formTitle`。早先这里只读
   * `formTitle`，结果是新建永远提示"分类名称不能为空"，改名则提交旧标题
   * ——后端 UPDATE 同值，提示成功但什么都没变。
   *
   * 首尾空白在这里去掉（后端也会再去一次）：空名不该发到网络上，
   * 用户看到的提示是"分类名称不能为空"而不是一次静默失败。
   */
  async function submit(title?: string) {
    if (saving.value) return;

    const raw = title ?? formTitle.value;
    const nextTitle = raw.trim();
    if (!nextTitle) {
      options.message.warning("分类名称不能为空");
      return;
    }

    const renaming = editingCategory.value;
    saving.value = true;
    try {
      await saveProfileCategory({ ...(renaming ? { id: renaming.id } : {}), title: nextTitle });
      formOpen.value = false;
      options.message.success(renaming ? "分类已改名" : "分类已新建");
      await options.reload();
    } catch (error) {
      // 「名称重复」等可读文案由后端给出，原样展示（ADR-0036 约束 5）。
      // 失败时弹窗保持打开：用户改个名字就能重试，不用重新敲。
      options.message.error(errorMessage(error, "保存分类失败"));
    } finally {
      saving.value = false;
    }
  }

  /**
   * 删除分类：先二次确认，并告知这一下会影响几篇文章。
   *
   * 没文章时也确认（删除不可逆），但不再说"0 篇文章会变成未分类"——
   * 那句话只会让人以为系统算错了。
   */
  function remove(category: ManageableCategory) {
    const count = category.article_count ?? 0;
    const content =
      count > 0
        ? `删除「${category.title}」后，该分类下的 ${count} 篇文章会变成「未分类」，文章本身不会删除。确认删除？`
        : `删除「${category.title}」？该分类下还没有文章。`;

    options.confirm({
      title: "删除分类",
      content,
      onConfirm: async () => {
        try {
          await deleteProfileCategories([category.id]);
          options.message.success("分类已删除");
          options.onDeleted?.(category.id);
          await options.reload();
        } catch (error) {
          options.message.error(errorMessage(error, "删除分类失败"));
        }
      },
    });
  }

  return {
    formOpen,
    formTitle,
    editingCategory,
    formMode,
    saving,
    openCreate,
    openRename,
    submit,
    remove,
  };
}
