import { onBeforeUnmount, onMounted, ref, toRef, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";

import { resolveActiveHeading, resolveProgressPercent } from "~/utils/readingProgress";

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

    // DOM 测量留在 composable，判定规则在 utils/readingProgress（可单测）。
    const tops: Record<string, number> = {};
    for (const id of ids) {
      const element = resolveHeadingElement?.(id) || document.getElementById(id);
      if (element) {
        tops[id] = element.getBoundingClientRect().top;
      }
    }

    activeHeadingId.value = resolveActiveHeading(ids, tops);
    progressPercent.value = resolveProgressPercent(
      window.scrollY || window.pageYOffset || 0,
      document.documentElement.scrollHeight,
      window.innerHeight,
    );
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
