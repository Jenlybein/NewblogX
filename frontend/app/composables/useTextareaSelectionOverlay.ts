import type { Ref } from "vue";
import { onMounted, onUnmounted, reactive } from "vue";

interface SelectionOverlayPosition {
  visible: boolean;
  top: number;
  left: number;
}

const TOOLBAR_HALF_WIDTH_PX = 176;
const TOOLBAR_GAP_PX = 54;
const VIEWPORT_MARGIN_PX = 12;

const MIRROR_STYLE_PROPS = [
  "boxSizing",
  "width",
  "height",
  "overflowX",
  "overflowY",
  "borderTopWidth",
  "borderRightWidth",
  "borderBottomWidth",
  "borderLeftWidth",
  "paddingTop",
  "paddingRight",
  "paddingBottom",
  "paddingLeft",
  "fontStyle",
  "fontVariant",
  "fontWeight",
  "fontStretch",
  "fontSize",
  "fontSizeAdjust",
  "lineHeight",
  "fontFamily",
  "textAlign",
  "textTransform",
  "textIndent",
  "textDecoration",
  "letterSpacing",
  "wordSpacing",
  "tabSize",
  "MozTabSize",
  "whiteSpace",
  "wordBreak",
  "overflowWrap",
  "wordWrap",
] as const;

/**
 * 把 textarea 的排版样式抄到镜像 div 上。
 *
 * 宽度**不抄 border-box 的 `width`**：textarea 是滚动容器，原生滚动条会占掉内容宽，
 * 按 border-box 宽度抄出来的镜像比正文实际可用宽度更宽 —— 换行位置跟着提前/推迟，
 * 选区在镜像里落到另一行，算出的锚点就会整体飘高（选中后弹窗不在选区正上方）。
 * 改用 `clientWidth / clientHeight`（已排除边框与滚动条），再配合 `border: 0`，
 * 镜像的文本宽度与 textarea 的完全一致。
 */
function copyTextareaStyles(textarea: HTMLTextAreaElement, mirror: HTMLDivElement) {
  const style = window.getComputedStyle(textarea);
  for (const prop of MIRROR_STYLE_PROPS) {
    if (prop === "width" || prop === "height" || prop.startsWith("border")) {
      continue;
    }
    mirror.style.setProperty(prop, style.getPropertyValue(prop));
  }

  mirror.style.position = "fixed";
  mirror.style.left = `${textarea.getBoundingClientRect().left}px`;
  mirror.style.top = `${textarea.getBoundingClientRect().top}px`;
  mirror.style.visibility = "hidden";
  mirror.style.pointerEvents = "none";
  mirror.style.zIndex = "-1";
  mirror.style.whiteSpace = "pre-wrap";
  mirror.style.wordWrap = "break-word";
  mirror.style.overflow = "hidden";
  mirror.style.boxSizing = "border-box";
  mirror.style.width = `${textarea.clientWidth}px`;
  mirror.style.height = `${textarea.clientHeight}px`;
  mirror.style.borderWidth = "0";
}

function buildSelectionMirror(textarea: HTMLTextAreaElement, selectionStart: number, selectionEnd: number) {
  const mirror = document.createElement("div");
  copyTextareaStyles(textarea, mirror);

  const before = textarea.value.slice(0, selectionStart);
  const selected = textarea.value.slice(selectionStart, selectionEnd);
  const after = textarea.value.slice(selectionEnd) || " ";

  mirror.textContent = before;

  const selectionSpan = document.createElement("span");
  selectionSpan.textContent = selected || " ";
  mirror.appendChild(selectionSpan);
  mirror.appendChild(document.createTextNode(after));

  document.body.appendChild(mirror);

  return {
    mirror,
    selectionSpan,
  };
}

function resolveSelectionAnchor(textarea: HTMLTextAreaElement, selectionStart: number, selectionEnd: number) {
  if (selectionEnd <= selectionStart) {
    return null;
  }

  const { mirror, selectionSpan } = buildSelectionMirror(textarea, selectionStart, selectionEnd);

  try {
    const mirrorRect = mirror.getBoundingClientRect();
    const selectionRect = selectionSpan.getBoundingClientRect();
    const textareaRect = textarea.getBoundingClientRect();

    // 镜像未滚动、textarea 已滚动：选区在**当前可视区**的纵向位置要减去 scrollTop。
    const contentTop = selectionRect.top - mirrorRect.top - textarea.scrollTop;
    const contentLeft = selectionRect.left - mirrorRect.left - textarea.scrollLeft;
    const centerX = contentLeft + Math.max(0, Math.min(selectionRect.width, textareaRect.width)) / 2;

    // 选区最后一行（用于上方放不下时改放下方）。
    const contentBottom = contentTop + Math.max(selectionRect.height, 0);

    return {
      top: textareaRect.top + contentTop,
      bottom: textareaRect.top + contentBottom,
      left: textareaRect.left + centerX,
    };
  } finally {
    mirror.remove();
  }
}

export function useTextareaSelectionOverlay(
  textareaRef: Ref<HTMLTextAreaElement | null>,
  options: {
    hideWhen?: () => boolean;
    ignoreSelectors?: string[];
  } = {},
) {
  const position = reactive<SelectionOverlayPosition>({
    visible: false,
    top: 0,
    left: 0,
  });

  function hide() {
    position.visible = false;
  }

  function updatePosition() {
    if (options.hideWhen?.()) {
      hide();
      return;
    }

    const textarea = textareaRef.value;
    if (!textarea) {
      hide();
      return;
    }

    const selectionStart = textarea.selectionStart ?? 0;
    const selectionEnd = textarea.selectionEnd ?? selectionStart;
    const selectedText = textarea.value.slice(selectionStart, selectionEnd);

    if (!selectedText.trim()) {
      hide();
      return;
    }

    const anchor = resolveSelectionAnchor(textarea, selectionStart, selectionEnd);
    if (!anchor) {
      hide();
      return;
    }

    // 悬浮条宽约 344px（四个按钮 + 内边距），只有中心对齐还不够：
    // 贴边选中时必须整体退回来，否则会被视口裁掉一半。
    const toolbarHalfWidth = TOOLBAR_HALF_WIDTH_PX;
    position.left = Math.max(
      toolbarHalfWidth + VIEWPORT_MARGIN_PX,
      Math.min(window.innerWidth - toolbarHalfWidth - VIEWPORT_MARGIN_PX, anchor.left),
    );

    // 默认浮在选区正上方；顶部放不下时改放到选区下方，而不是被挤在视口边缘。
    const above = anchor.top - TOOLBAR_GAP_PX;
    position.top = above >= VIEWPORT_MARGIN_PX ? above : anchor.bottom + VIEWPORT_MARGIN_PX;
    position.visible = true;
  }

  function handleDocumentPointerDown(event: PointerEvent) {
    const textarea = textareaRef.value;
    const target = event.target as Node | null;
    const targetElement = event.target instanceof Element ? event.target : null;

    if (!textarea || !target) {
      hide();
      return;
    }

    if (textarea === target || textarea.contains(target)) {
      return;
    }

    if (targetElement && options.ignoreSelectors?.some((selector) => targetElement.closest(selector))) {
      return;
    }

    hide();
  }

  onMounted(() => {
    window.addEventListener("pointerdown", handleDocumentPointerDown, true);
    window.addEventListener("resize", updatePosition);
  });

  onUnmounted(() => {
    window.removeEventListener("pointerdown", handleDocumentPointerDown, true);
    window.removeEventListener("resize", updatePosition);
  });

  return {
    selectionOverlay: position,
    updateSelectionOverlay: updatePosition,
    hideSelectionOverlay: hide,
  };
}
