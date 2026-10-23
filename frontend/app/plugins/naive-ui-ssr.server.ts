import { setup } from "@css-render/vue3-ssr";

const STYLE_TAG_PATTERN = /<style[^>]*cssr-id="([^"]+)"[^>]*>([\s\S]*?)<\/style>/g;

export default defineNuxtPlugin({
  name: "naive-ui-ssr",
  enforce: "pre",
  setup(nuxtApp) {
    const { collect } = setup(nuxtApp.vueApp);

    nuxtApp.ssrContext?.head.hooks.hook("tags:resolve", (ctx) => {
      const styleTags = Array.from(collect().matchAll(STYLE_TAG_PATTERN)).flatMap((match) => {
        const cssrId = match[1];
        const innerHTML = match[2];

        if (!cssrId || !innerHTML) {
          return [];
        }

        return [{
          tag: "style" as const,
          props: {
            "cssr-id": cssrId,
          },
          innerHTML: innerHTML.trim(),
        }];
      });

      if (!styleTags.length) {
        return;
      }

      const lastMetaIndex = ctx.tags.map((tag) => tag.tag).lastIndexOf("meta");
      const insertIndex = lastMetaIndex >= 0 ? lastMetaIndex + 1 : 0;
      ctx.tags.splice(insertIndex, 0, ...styleTags);
    });
  },
});
