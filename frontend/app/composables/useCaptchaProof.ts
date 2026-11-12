import { computed, ref, watch } from "vue";
import { getImageCaptcha, type CaptchaProof } from "~/services/auth";
import { useSiteStore } from "~/stores/site";

/**
 * 图形验证码的状态：图片、输入、以及"这次要不要带"。
 *
 * 四个入口共用（邮箱发码 / 短信发码 / 邮箱注册发码 / 密码登录），它们后端都挂了
 * `middleware.CaptchaMiddleware`，读的都是请求体里的 `captcha_id` + `captcha_code`。
 * 开关在 `site.login.captcha`，关着时整个控件不渲染、请求体也不带这两个字段。
 *
 * 注意验证码是**一次性**的：`ImageCaptchaStore.Verify(..., true)` 校验成功即删，
 * 所以每次提交之后都要 `refresh()`，否则第二次一定失败。
 */
export function useCaptchaProof() {
  const siteStore = useSiteStore();

  const enabled = computed(() => siteStore.runtimeConfig?.login?.captcha === true);
  const image = ref("");
  const id = ref("");
  const code = ref("");
  const loading = ref(false);

  async function refresh() {
    if (!enabled.value) {
      image.value = "";
      id.value = "";
      code.value = "";
      return;
    }

    loading.value = true;
    try {
      const data = await getImageCaptcha();
      image.value = data.base64;
      id.value = data.captcha_id;
      // 换了图，之前的输入作废：旧字母对应的是上一张图。
      code.value = "";
    } catch {
      // 这里不弹提示：该 composable 出现在四个表单里，弹窗会重复。
      // 取不到图时不带验证码提交，由服务端给出一次明确的错误。
      image.value = "";
      id.value = "";
    } finally {
      loading.value = false;
    }
  }

  // 站点配置是异步取回的：开关一到位就去取图（开关本来就关着时不会发请求）。
  watch(
    enabled,
    (on) => {
      if (on) {
        void refresh();
      }
    },
    { immediate: true },
  );

  /** 站点开了验证码但没填齐：本地先挡一道，省一次必然失败的请求。 */
  const missing = computed(() => enabled.value && (!id.value || !code.value.trim()));

  /** 要摊进请求体的字段；开关关着或没填齐时返回 undefined（不带这两个键）。 */
  function payload(): CaptchaProof | undefined {
    if (!enabled.value || !id.value) {
      return undefined;
    }
    const value = code.value.trim();
    if (!value) {
      return undefined;
    }
    return { captcha_id: id.value, captcha_code: value };
  }

  return { enabled, image, code, loading, missing, refresh, payload };
}
