<script setup lang="ts" generic="T extends string = string">
/**
 * 多列 chip 下拉（票 352）。
 *
 * 为什么不用 NSelect 自带菜单：它的菜单是**竖向虚拟列表**，一行一个选项。
 * 标签词库有 56 条，发布文章时"往下翻着找"是主要痛点；把选项排成流式 chip
 * 网格后一屏能看到几十个，点一下即选。
 *
 * 行为刻意保持与原生下拉一致的部分：
 *  - 点触发器开合，点面板外关闭（NPopover 的 clickoutside）；
 *  - 面板内有搜索框，输入即过滤；
 *  - 上下键移动高亮、回车选中、ESC 关闭；
 *  - 多选时已选项在面板内显示选中态，触发器上以 tag 形式回显。
 *
 * 刻意**不做**的两件事（产品决定）：
 *  - 不提供"无匹配时新建"：新建入口只保留在业务页面里（例如"+ 新建分类"）；
 *  - 不在选项上显示文章数：chip 里只放名称，密度优先。
 */
import { computed, nextTick, ref, watch } from 'vue'
import { NInput, NPopover, NTag } from 'naive-ui'

interface ChipOption<V extends string = string> {
  label: string
  value: V
}

const props = withDefaults(defineProps<{
  options: ChipOption<T>[]
  multiple?: boolean
  placeholder?: string
  clearable?: boolean
  /** 触发器上最多显示几个已选 tag，其余折成 +N。 */
  maxTagCount?: number
  disabled?: boolean
  /** 用于测试与可访问性：触发器与面板的稳定标识。 */
  dataRole?: string
}>(), {
  multiple: false,
  placeholder: '请选择',
  clearable: true,
  maxTagCount: 3,
  disabled: false,
  dataRole: 'chip-select',
})

/** 单选绑字符串、多选绑字符串数组：与 NSelect 的 v-model 语义一致。 */
const model = defineModel<T | T[] | null>({ default: null })

const open = ref(false)
const keyword = ref('')
const activeIndex = ref(0)
const triggerRef = ref<HTMLElement | null>(null)

const selectedValues = computed<T[]>(() => {
  const value = model.value
  if (Array.isArray(value)) return value
  if (value === null || value === undefined || value === '') return []
  return [value as T]
})

const selectedSet = computed(() => new Set(selectedValues.value))

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return props.options
  return props.options.filter(option => option.label.toLowerCase().includes(kw))
})

/** 触发器上显示的文字（单选）或 tag 列表（多选）。 */
const selectedLabels = computed(() =>
  selectedValues.value
    .map(value => props.options.find(option => option.value === value)?.label ?? value)
    .filter(Boolean))

const visibleTags = computed(() => selectedLabels.value.slice(0, props.maxTagCount))
const hiddenTagCount = computed(() => Math.max(0, selectedLabels.value.length - props.maxTagCount))

function toggleOpen() {
  if (props.disabled) return
  open.value = !open.value
}

watch(open, (value) => {
  if (value) {
    keyword.value = ''
    activeIndex.value = 0
    return
  }
})

// 搜索后高亮回到第一条：否则回车会选中一条当前看不见的选项。
watch(keyword, () => {
  activeIndex.value = 0
})

function isSelected(value: T) {
  return selectedSet.value.has(value)
}

function pick(option: ChipOption<T>) {
  if (props.multiple) {
    const next = new Set(selectedValues.value)
    if (next.has(option.value)) next.delete(option.value)
    else next.add(option.value)
    model.value = [...next] as T[]
    return
  }
  model.value = option.value
  open.value = false
}

function clearAll() {
  model.value = props.multiple ? [] : null
}

function moveActive(step: number) {
  const total = filtered.value.length
  if (total === 0) return
  activeIndex.value = (activeIndex.value + step + total) % total
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    if (!open.value) {
      open.value = true
      return
    }
    moveActive(event.key === 'ArrowDown' ? 1 : -1)
    return
  }
  if (event.key === 'Enter') {
    if (!open.value) {
      open.value = true
      event.preventDefault()
      return
    }
    const option = filtered.value[activeIndex.value]
    if (option) {
      event.preventDefault()
      pick(option)
    }
    return
  }
  if (event.key === 'Escape' && open.value) {
    event.preventDefault()
    open.value = false
  }
}

/** 面板打开后把焦点交给搜索框：打开即可输入，不用再点一次。 */
const searchRef = ref<InstanceType<typeof NInput> | null>(null)
watch(open, async (value) => {
  if (!value) return
  await nextTick()
  const input = (searchRef.value as unknown as { $el?: HTMLElement } | null)?.$el
  input?.querySelector('input')?.focus()
})
</script>

<template>
  <NPopover
    v-model:show="open"
    trigger="manual"
    placement="bottom-start"
    :disabled="disabled"
    :show-arrow="false"
    raw
    class="chip-select__popover">
    <template #trigger>
      <button
        ref="triggerRef"
        type="button"
        class="chip-select__trigger"
        :class="{ 'chip-select__trigger--open': open, 'chip-select__trigger--empty': selectedLabels.length === 0 }"
        :disabled="disabled"
        :data-role="`${dataRole}-trigger`"
        :aria-expanded="open"
        aria-haspopup="listbox"
        @click="toggleOpen"
        @keydown="onKeydown">
        <template v-if="selectedLabels.length === 0">
          <span class="chip-select__placeholder">{{ placeholder }}</span>
        </template>
        <template v-else-if="multiple">
          <NTag v-for="label in visibleTags" :key="label" size="small" :bordered="false">{{ label }}</NTag>
          <span v-if="hiddenTagCount > 0" class="chip-select__more">+{{ hiddenTagCount }}</span>
        </template>
        <template v-else>
          <span class="chip-select__value">{{ selectedLabels[0] }}</span>
        </template>

        <span v-if="clearable && selectedLabels.length > 0" class="chip-select__clear" @click.stop="clearAll">✕</span>
        <span class="chip-select__caret" aria-hidden="true">▾</span>
      </button>
    </template>

    <div class="chip-select__panel" :data-role="`${dataRole}-panel`">
      <NInput
        ref="searchRef"
        v-model:value="keyword"
        size="small"
        placeholder="搜索"
        clearable
        :data-role="`${dataRole}-search`"
        @keydown="onKeydown" />

      <div v-if="filtered.length === 0" class="chip-select__empty" :data-role="`${dataRole}-empty`">
        没有匹配项
      </div>
      <div v-else class="chip-select__chips" role="listbox" :aria-multiselectable="multiple">
        <button
          v-for="(option, index) in filtered"
          :key="option.value"
          type="button"
          class="chip-select__chip"
          :class="{
            'chip-select__chip--selected': isSelected(option.value),
            'chip-select__chip--active': index === activeIndex,
          }"
          role="option"
          :aria-selected="isSelected(option.value)"
          :data-role="`${dataRole}-option`"
          @click="pick(option)"
          @mouseenter="activeIndex = index">
          {{ option.label }}
        </button>
      </div>
    </div>
  </NPopover>
</template>
