<script setup lang="ts">
/**
 * 站点 → 图形验证码（`/site/captcha`，ADR-0033）。
 *
 * 这一页管第 12 段运行时配置 `captcha`：图形与时效参数，也就是"那张图长什么样、
 * 多久失效"。三件事必须一起说清楚，否则运营会在错误的位置找开关：
 *
 *   1. **总开关与四个入口闸门都在本页**（票 票 481 把它们从 `site.login` 拆进 `captcha`
 *      段）：图形码开关与"图片长什么样"是同一件事的两面，住在两页时想只给发码加
 *      验证码要翻半页表；
 *   2. **读者站不再读这一段**：它有凭据以外的运营参数，也不公开；读者站改读
 *      公开只读端点 `GET /api/site/login_capabilities`（它把总开关与四个闸门合取后
 *      只回布尔位，不含任何参数）；
 *   3. **保存即生效**：生成请求每次都按现值构建驱动、按现值写有效期，不需要重启。
 *
 * 字段与驱动的适用关系一栏由前端标出来：不适用项直接禁用输入框，并在旁边写明
 * "该类型不使用"。注意后端的清零范围是**有限**的：source / bg_color / noise_count /
 * 三个干扰线开关按驱动清零；max_skew / dot_count 保留原值（清掉会让 GET→PUT
 * 往返漂移，见 ADR-0033 决策 13）。
 */
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NRadioButton,
  NRadioGroup,
  NSpace,
  NSwitch,
  useMessage,
} from 'naive-ui'
import { getCaptchaRuntimeConfig, updateCaptchaRuntimeConfig } from '~/services/admin'
import { isApiBusinessError } from '~/services/http/errors'
import type { CaptchaRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
const loaded = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染。
const config = ref({} as CaptchaRuntimeConfig)

/** 三种驱动。文案按"读者实际看到什么"写，不写库里的类型名。 */
const DRIVER_OPTIONS = [
  { label: '字符型（字母 + 数字）', value: 'string' as const },
  { label: '算术型（两位数运算）', value: 'math' as const },
  { label: '纯数字', value: 'digit' as const },
]

/** 当前驱动决定哪些字段真的生效（后端会把不适用项清零，界面先禁用）。 */
const isString = computed(() => config.value.driver === 'string')
const isMath = computed(() => config.value.driver === 'math')
const isDigit = computed(() => config.value.driver === 'digit')
/** 字符型 / 算术型才有的图形参数（噪点、干扰线、背景色）。 */
const hasTextGraphics = computed(() => isString.value || isMath.value)
/** 字符型 / 纯数字才有"字符数"（算术型的题目固定是两位数运算）。 */
const hasLength = computed(() => !isMath.value)

async function load() {
  loading.value = true
  try {
    config.value = await getCaptchaRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('图形验证码配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateCaptchaRuntimeConfig(config.value)
    // 保存后立刻回读：后端归一化会补齐默认值、清零不适用项，
    // 不回读的话界面还停留在用户填的值，看起来"保存没生效"。
    config.value = await getCaptchaRuntimeConfig()
    message.success('图形验证码配置已保存')
  } catch (error) {
    // 越界时后端带回的是"图片宽度需在 80–400 之间，收到 5000"这种字段级文案，
    // 必须原样显示：只说"保存失败"会让运营反复试同一组数字。
    // 取不到业务文案时退回一句能行动的话（不是"保存失败"了事）。
    message.error(isApiBusinessError(error) ? error.message : '保存失败：配置超出允许范围，请检查填入的数值')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="admin-settings-panel">
    <NAlert type="info" :bordered="false" class="mb-4">
      本页管<strong>总开关、哪些入口要验证码、图片长什么样</strong>。
      读者站不读这里，它读公开只读端点 <code>GET /api/site/login_capabilities</code> ——
      那个端点把总开关与四个闸门合取后只回布尔位，不带任何图片参数。
    </NAlert>
    <NAlert type="warning" :bordered="false" class="mb-4">
      修改<strong>有效期</strong>只影响之后新生成的图：已经发到用户浏览器里的那张，
      仍按它生成时的有效期过期（不会把人手里的图当场作废）。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="开关与入口" size="small">
        <NFormItem label="启用图形验证码（总开关）">
          <NSwitch v-model:value="config.captcha" data-role="captcha-master-switch" />
        </NFormItem>
        <!--
          四个入口闸门：总开关打开后，逐个决定哪条路由真的校验。
          登录与发码被滥用的方式不同（发码烧钱、登录被撞库），一刀切会让
          「只想给发码加验证码」做不到。
        -->
        <NFormItem label="登录接口要求图形验证码">
          <NSwitch
            v-model:value="config.captcha_login"
            :disabled="!config.captcha"
            data-role="captcha-gate-login"
          />
          <span v-if="!config.captcha" class="admin-hint">总开关关闭时不生效</span>
        </NFormItem>
        <NFormItem label="发邮箱验证码要求图形验证码">
          <NSwitch
            v-model:value="config.captcha_email_verify"
            :disabled="!config.captcha"
            data-role="captcha-gate-email"
          />
        </NFormItem>
        <NFormItem label="发短信验证码要求图形验证码">
          <NSwitch
            v-model:value="config.captcha_phone_verify"
            :disabled="!config.captcha"
            data-role="captcha-gate-phone"
          />
        </NFormItem>
        <NFormItem label="注册接口要求图形验证码">
          <NSwitch
            v-model:value="config.captcha_register"
            :disabled="!config.captcha"
            data-role="captcha-gate-register"
          />
          <span class="admin-hint">只作用于专门的注册路由；手机号注册的验证码在发码那一步</span>
        </NFormItem>
      </NCard>

      <NFormItem label="验证码类型">
        <NRadioGroup v-model:value="config.driver">
          <NRadioButton
            v-for="option in DRIVER_OPTIONS"
            :key="option.value"
            :value="option.value"
            data-role="captcha-driver"
          >
            {{ option.label }}
          </NRadioButton>
        </NRadioGroup>
      </NFormItem>

      <NSpace :size="24">
        <NFormItem label="字符数">
          <NInputNumber
            v-model:value="config.length"
            class="admin-field--number"
            :disabled="!hasLength"
            :min="isDigit ? 3 : 2"
            :max="8"
            data-role="captcha-length"
          />
          <span v-if="!hasLength" class="admin-hint">算术型不使用此项</span>
        </NFormItem>
        <NFormItem label="图片宽度（像素）">
          <NInputNumber
            v-model:value="config.width"
            class="admin-field--number"
            :min="80"
            :max="400"
            data-role="captcha-width"
          />
        </NFormItem>
        <NFormItem label="图片高度（像素）">
          <NInputNumber
            v-model:value="config.height"
            class="admin-field--number"
            :min="30"
            :max="120"
            data-role="captcha-height"
          />
        </NFormItem>
        <NFormItem label="有效期（秒）">
          <NInputNumber
            v-model:value="config.expire_seconds"
            class="admin-field--number"
            :min="60"
            :max="3600"
            data-role="captcha-expire"
          />
        </NFormItem>
      </NSpace>

      <NFormItem label="字符集">
        <NInput
          v-model:value="config.source"
          class="admin-field--long"
          :disabled="!isString"
          data-role="captcha-source"
        />
        <span v-if="!isString" class="admin-hint">仅字符型使用此字符集</span>
        <span v-else class="admin-hint">
          保存时会去掉空白与重复字符；去重后不足 2 个字符会被拒绝。
          默认已去掉易混的 0 / O / 1 / I / L。
        </span>
      </NFormItem>

      <NFormItem label="填错时的提示文案">
        <NInput
          v-model:value="config.fail_message"
          class="admin-field--long"
          :maxlength="80"
          placeholder="留空使用默认文案：图形验证码错误"
          data-role="captcha-fail-message"
        />
        <span class="admin-hint">
          读者填错验证码时看到的那句话。留空表示用内置默认文案；最多 80 个字符。
        </span>
      </NFormItem>

      <NSpace :size="24">
        <NFormItem label="噪点数量">
          <NInputNumber
            v-model:value="config.noise_count"
            class="admin-field--number"
            :disabled="!hasTextGraphics"
            :min="0"
            :max="10"
            data-role="captcha-noise"
          />
          <span v-if="!hasTextGraphics" class="admin-hint">纯数字不使用此项</span>
        </NFormItem>
        <NFormItem label="背景色">
          <NInput
            v-model:value="config.bg_color"
            class="admin-field--long"
            :disabled="!hasTextGraphics"
            data-role="captcha-bg"
          />
          <span v-if="!hasTextGraphics" class="admin-hint">纯数字不使用此项</span>
          <span v-else class="admin-hint">支持 #RRGGBB 或 #RRGGBBAA</span>
        </NFormItem>
      </NSpace>

      <NCard size="small" class="mb-4" title="干扰线">
        <span class="admin-hint mb-2 block">
          三种线各自一个开关。开得越多越难识别，也越可能让人自己看错。
        </span>
        <NSpace :size="24">
          <NFormItem label="空心线">
            <NSwitch
              v-model:value="config.show_hollow_line"
              :disabled="!hasTextGraphics"
              data-role="captcha-hollow-line"
            />
          </NFormItem>
          <NFormItem label="泥浆线">
            <NSwitch
              v-model:value="config.show_slime_line"
              :disabled="!hasTextGraphics"
              data-role="captcha-slime-line"
            />
          </NFormItem>
          <NFormItem label="正弦线">
            <NSwitch
              v-model:value="config.show_sine_line"
              :disabled="!hasTextGraphics"
              data-role="captcha-sine-line"
            />
          </NFormItem>
        </NSpace>
      </NCard>

      <NSpace :size="24">
        <NFormItem label="最大倾斜">
          <NInputNumber
            v-model:value="config.max_skew"
            class="admin-field--number"
            :disabled="!isDigit"
            :min="0"
            :max="1"
            :step="0.1"
            data-role="captcha-max-skew"
          />
          <span v-if="!isDigit" class="admin-hint">仅纯数字使用</span>
        </NFormItem>
        <NFormItem label="背景点数">
          <NInputNumber
            v-model:value="config.dot_count"
            class="admin-field--number"
            :disabled="!isDigit"
            :min="0"
            :max="20"
            data-role="captcha-dot-count"
          />
          <span v-if="!isDigit" class="admin-hint">仅纯数字使用</span>
        </NFormItem>
      </NSpace>

      <NButton type="primary" :loading="saving" data-role="captcha-save" @click="save">
        保存图形验证码配置
      </NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>

