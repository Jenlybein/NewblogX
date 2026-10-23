import type { Ref } from "vue";
import { onMounted, onUnmounted, reactive } from "vue";

interface SelectionOverlayPosition {
  visible: boolean;
  top: number;
  left: number;
}

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

function copyTextareaStyles(textarea: HTMLTextAreaElement, mirror: HTMLDivElement) {
  const style = window.getComputedStyle(textarea);
  for (const prop of MIRROR_STYLE_PROPS) {
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

    const contentTop = selectionRect.top - mirrorRect.top - textarea.scrollTop;
    const contentLeft = selectionRect.left - mirrorRect.left - textarea.scrollLeft;
    const centerX = contentLeft + Math.max(0, Math.min(selectionRect.width, textareaRect.width)) / 2;

    return {
      top: textareaRect.top + contentTop,
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

    position.visible = true;
    position.left = Math.max(24, Math.min(window.innerWidth - 24, anchor.left));
    position.top = Math.max(16, anchor.top - 54);
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
