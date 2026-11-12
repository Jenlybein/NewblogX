import { computed, onScopeDispose, ref } from "vue";

/** 验证码按钮的默认锁定时间（秒），与后端发码限流窗口对齐。 */
const DEFAULT_SECONDS = 60;

/**
 * 验证码按钮的倒数计时：发一次、锁 N 秒。
 *
 * 这四个表单（邮箱登录 / 邮箱注册 / 手机号登录 / 后台首设密码）原本各写一份
 * `setInterval` + `clearInterval` + 一个 `sendDisabled` computed，改一处就会漏三处；
 * 收成一份之后，唯一要记的是"发成功才 start()"。
 *
 * 作用域销毁时自动停表：否则组件卸载后计时器还在跑，回调里去改一个已经没人看的 ref。
 */
export function useVerificationCountdown(seconds = DEFAULT_SECONDS) {
  const remaining = ref(0);
  let timer: ReturnType<typeof setInterval> | null = null;

  function stop() {
    if (timer) {
      clearInterval(timer);
      timer = null;
    }
  }

  /** 发送成功后调用：从 seconds 开始倒数。 */
  function start() {
    stop();
    remaining.value = seconds;
    timer = setInterval(() => {
      remaining.value -= 1;
      if (remaining.value <= 0) {
        stop();
      }
    }, 1000);
  }

  /** 重新打开弹窗时归零。 */
  function reset() {
    stop();
    remaining.value = 0;
  }

  onScopeDispose(stop);

  const counting = computed(() => remaining.value > 0);
  /** 按钮文案：倒数时显示剩余秒数，否则提示可发。 */
  const sendLabel = computed(() => (remaining.value > 0 ? `${remaining.value}s` : "发送验证码"));

  return { remaining, counting, sendLabel, start, reset };
}
