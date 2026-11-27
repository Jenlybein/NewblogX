import { computed, onScopeDispose, ref, watch } from "vue";
import { getImageCaptcha, type CaptchaProof } from "~/services/auth";
import { useSiteStore } from "~/stores/site";

/**
 * 图形验证码的入口标识，与后端 `middleware.CaptchaScope*` 一一对应。
 *
 * 四个入口各自一个闸门（`captcha.captcha_login` 等），因为登录与发码被滥用的
 * 方式不同：发码烧钱（短信按条计费），登录被撞库。一刀切会让"只想给发码加验证码"
 * 做不到，而真的只想加一处时又只能整体关掉、连带拆掉另一处的防护。
 */
export type CaptchaGateScope = "login" | "email_verify" | "phone_verify" | "register";

/**
 * 图形验证码的状态：图片、输入、以及"这次要不要带"。
 *
 * 四个表单共用（密码登录 / 邮箱发码 / 短信发码 / 邮箱注册发码），但**各自传自己的
 * scope**：后端按入口校验，前端也要按入口渲染 —— 否则会出现"登录表单画了框、
 * 但后端那条路由不校验"的错位（用户白填，还以为自己填错了）。
 *
 * 是否启用 = 后端给出的**生效值**（`GET /api/site/login_capabilities` 的
 * `captcha_required`）。总开关与入口闸门在后端已经合取过，前端不再自己 `&&`
 * 一次：每个调用点各与一次，漏一处就渲染出一个永远不被校验的输入框。
 *
 * 注意验证码是**一次性**的：存储层校验成功即删（`Verify(..., true)`），
 * 所以每次提交之后都要 `refresh()`，否则第二次一定失败。
 */
export function useCaptchaProof(scope: CaptchaGateScope = "login") {
  const siteStore = useSiteStore();

  const enabled = computed(() => {
    const required = siteStore.loginCapabilities?.captcha_required;
    if (!required) return false;
    switch (scope) {
      case "email_verify":
        return required.email_verify === true;
      case "phone_verify":
        return required.phone_verify === true;
      case "register":
        return required.register === true;
      default:
        return required.login === true;
    }
  });

  const image = ref("");
  const id = ref("");
  const code = ref("");
  const loading = ref(false);
  /** 有效期（秒），由后端回显：页面据此知道多久没操作就该主动换一张。 */
  const expireSeconds = ref(0);

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
      expireSeconds.value = Number(data.expire_seconds ?? 0) || 0;
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

  // 站点配置是异步取回的：闸门一到位就去取图（没开时不会发请求）。
  watch(
    enabled,
    (on) => {
      if (on) {
        void refresh();
      }
    },
    { immediate: true },
  );

  // 按后端回显的有效期主动换图：一张图放久了，用户看到的是"填对了却过期"。
  //
  // 留 5 秒安全余量而不是卡点换：正好在过期那一刻替换，用户可能刚好提交了旧图，
  // 服务端已经删了它 —— 那是最难解释的一种失败（"我明明填对了"）。
  let expireTimer: ReturnType<typeof setTimeout> | undefined;
  function scheduleExpireRefresh() {
    if (expireTimer) {
      clearTimeout(expireTimer);
      expireTimer = undefined;
    }
    if (!enabled.value || expireSeconds.value <= 0) {
      return;
    }
    const delay = Math.max((expireSeconds.value - 5) * 1000, 10_000);
    expireTimer = setTimeout(() => void refresh(), delay);
  }
  watch([enabled, expireSeconds], () => scheduleExpireRefresh(), { immediate: true });
  onScopeDispose(() => {
    if (expireTimer) clearTimeout(expireTimer);
  });

  /** 该入口开了验证码但没填齐：本地先挡一道，省一次必然失败的请求。 */
  const missing = computed(() => enabled.value && (!id.value || !code.value.trim()));

  /** 要摊进请求体的字段；没开或没填齐时返回 undefined（不带这两个键）。 */
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

  return { enabled, image, code, loading, missing, refresh, payload, expireSeconds };
}

