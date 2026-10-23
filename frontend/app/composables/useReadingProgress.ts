import { onBeforeUnmount, onMounted, ref, toRef, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";

type HeadingResolver = (id: string) => HTMLElement | null;

export function useReadingProgress(headingIds: MaybeRefOrGetter<string[]>, resolveHeadingElement?: HeadingResolver) {
  const headingIdsRef = toRef(headingIds);
  const activeHeadingId = ref("");
  const progressPercent = ref(0);
  let frameHandle = 0;

  function measure() {
    if (!import.meta.client) {
      return;
    }

    const ids = toValue(headingIdsRef);
    if (!ids.length) {
      activeHeadingId.value = "";
      progressPercent.value = 0;
      return;
    }

    const threshold = 164;
    let currentActive = ids[0] || "";

    for (const id of ids) {
      const element = resolveHeadingElement?.(id) || document.getElementById(id);
      if (!element) {
        continue;
      }

      if (element.getBoundingClientRect().top - threshold <= 0) {
        currentActive = id;
      } else {
        break;
      }
    }

    activeHeadingId.value = currentActive;

    const scrollTop = window.scrollY || window.pageYOffset || 0;
    const maxScrollable = Math.max(document.documentElement.scrollHeight - window.innerHeight, 0);
    progressPercent.value = maxScrollable > 0 ? Math.min(100, Math.max(0, (scrollTop / maxScrollable) * 100)) : 0;
  }

  function scheduleMeasure() {
    if (!import.meta.client) {
      return;
    }
    cancelAnimationFrame(frameHandle);
    frameHandle = window.requestAnimationFrame(measure);
  }

  onMounted(() => {
    scheduleMeasure();
    window.addEventListener("scroll", scheduleMeasure, { passive: true });
    window.addEventListener("resize", scheduleMeasure, { passive: true });
  });

  onBeforeUnmount(() => {
    if (!import.meta.client) {
      return;
    }
    cancelAnimationFrame(frameHandle);
    window.removeEventListener("scroll", scheduleMeasure);
    window.removeEventListener("resize", scheduleMeasure);
  });

  watch(
    headingIdsRef,
    () => {
      scheduleMeasure();
    },
    { flush: "post" },
  );

  return {
    activeHeadingId,
    progressPercent,
  };
}
