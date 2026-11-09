<script setup lang="ts">
import { h } from "vue";
import { NButton, NCard, NDataTable, NSelect, NSpace, NTag, useDialog, useMessage } from "naive-ui";

import BannerFormModal from "~/components/media/BannerFormModal.vue";
import { deleteBanners, deleteImages, getBanners, getImages } from "~/services/admin";
import type { BannerModel, ImageListItem } from "~/types/api";
import { describeBannerCoverSize, readImageSize } from "~/utils/bannerCover";
import type { ImageSize } from "~/utils/bannerCover";
import { bannerTitleCell } from "~/utils/bannerTitle";
import { BANNER_SHOW_FILTER_OPTIONS, buildBannerListQuery } from "~/utils/bannerList";
import type { BannerShowFilter } from "~/utils/bannerList";
import { formatDateTime } from "~/utils/format";
import { hasNextPage } from "~/utils/pagination";

/**
 * 媒体页：轮播运营 + 图片资源库。
 *
 * 「新增轮播」是一个按钮 + 弹窗（票 330），不再是常驻在版面上的大卡片：
 * 以前它一直占着左侧一栏，而右侧列表里能做的只有「编辑标题」——建完之后的
 * 封面、跳转地址、展示状态全都改不了，只能删了重建。
 *
 * 现在新增与编辑复用同一个 `BannerFormModal`：字段一致、封面流程一致，
 * 差别只有标题与提交到的接口。本页只负责列表、筛选、删除与打开弹窗。
 */
const message = useMessage();
const dialog = useDialog();

const loadingBanners = ref(false);
const loadingImages = ref(false);
const BANNER_PAGE_SIZE = 10;
const IMAGE_PAGE_SIZE = 10;
const bannerPage = ref(1);
const bannerShowFilter = ref<BannerShowFilter>("all");
const imagePage = ref(1);
const hasMoreBanners = ref(false);
const hasMoreImages = ref(false);
const imageTotal = ref(0);
const banners = ref<BannerModel[]>([]);
const images = ref<ImageListItem[]>([]);

/** 新增 / 编辑共用的弹窗状态：mode 决定标题与提交到的接口。 */
const editorVisible = ref(false);
const editorMode = ref<"create" | "edit">("create");
const editorTarget = ref<BannerModel | null>(null);

function openCreateEditor() {
  editorMode.value = "create";
  editorTarget.value = null;
  editorVisible.value = true;
}

function openEditEditor(row: BannerModel) {
  editorMode.value = "edit";
  editorTarget.value = row;
  editorVisible.value = true;
}

/** 列表里每张封面的尺寸（按封面地址缓存）；null 表示加载失败，不再重试。 */
const coverSizes = ref<Record<string, ImageSize | null>>({});
/** 同一张封面的并发测量只发一次，避免翻页时重复解码。 */
const coverSizeRequests = new Map<string, Promise<ImageSize | null>>();

/**
 * 列表里的封面尺寸读数。
 *
 * 尺寸是从封面地址解码出来的（BannerItem 契约没有宽高字段），所以刚加载时是
 * "读取中…"；读不到显示"尺寸未知"，不拿 0×0 冒充。
 */
function renderCoverSize(row: BannerModel) {
  const cached =
    row.cover && Object.prototype.hasOwnProperty.call(coverSizes.value, row.cover)
      ? coverSizes.value[row.cover]
      : undefined;
  if (cached === undefined) {
    return h("span", { class: "admin-cover-size__muted" }, "读取中…");
  }

  const info = describeBannerCoverSize(cached);
  if (!info) {
    return h("span", { class: "admin-cover-size__muted" }, "尺寸未知");
  }
  return h("div", { class: "admin-cover-size" }, [h("span", { class: "admin-cover-size__pixels" }, info.label)]);
}

const bannerColumns = [
  {
    title: "封面",
    key: "cover",
    width: 180,
    render: (row: BannerModel) =>
      h("div", { class: "admin-cover" }, row.cover ? [h("img", { src: row.cover, alt: row.href })] : []),
  },
  {
    title: "标题",
    key: "title",
    minWidth: 200,
    render: (row: BannerModel) => {
      const text = bannerTitleCell(row.title);
      const empty = text === "未填写";
      return h("span", { class: empty ? "admin-cover-size__muted" : undefined }, text);
    },
  },
  { title: "跳转地址", key: "href", minWidth: 260 },
  {
    title: "展示",
    key: "show",
    width: 100,
    render: (row: BannerModel) =>
      h(NTag, { type: row.show ? "success" : "default", size: "small" }, { default: () => (row.show ? "展示" : "隐藏") }),
  },
  {
    title: "封面尺寸",
    key: "cover_size",
    width: 190,
    render: (row: BannerModel) => renderCoverSize(row),
  },
  {
    title: "更新时间",
    key: "updated_at",
    width: 180,
    render: (row: BannerModel) => formatDateTime(row.updated_at),
  },
  {
    title: "操作",
    key: "actions",
    width: 150,
    render: (row: BannerModel) =>
      h(NSpace, { size: 4, justify: "end" }, {
        default: () => [
          h(
            NButton,
            { size: "small", tertiary: true, "data-role": `edit-banner-${row.id}`, onClick: () => openEditEditor(row) },
            { default: () => "编辑" },
          ),
          h(
            NButton,
            { size: "small", tertiary: true, type: "error", "data-role": `delete-banner-${row.id}`, onClick: () => removeBanner(row) },
            { default: () => "删除" },
          ),
        ],
      }),
  },
];

const imageColumns = [
  { title: "ID", key: "id", minWidth: 220, render: (row: ImageListItem) => String(row.id || "-") },
  {
    title: "预览",
    key: "url",
    width: 180,
    render: (row: ImageListItem) => {
      const src = String(row.url || "");
      return h("div", { class: "admin-cover" }, src ? [h("img", { src, alt: String(row.file_name || row.id || "") })] : []);
    },
  },
  { title: "状态", key: "status", width: 120, render: (row: ImageListItem) => h(NTag, { size: "small" }, { default: () => String(row.status ?? "-") }) },
  {
    title: "创建时间",
    key: "created_at",
    width: 180,
    render: (row: ImageListItem) => formatDateTime(String(row.created_at || "")),
  },
  {
    title: "操作",
    key: "actions",
    width: 100,
    render: (row: ImageListItem) =>
      h(
        NButton,
        { size: "small", tertiary: true, type: "error", disabled: !row.id, onClick: () => removeImage(row) },
        { default: () => "删除" },
      ),
  },
];

async function refreshBanners() {
  loadingBanners.value = true;
  try {
    const data = await getBanners(buildBannerListQuery(bannerPage.value, BANNER_PAGE_SIZE, bannerShowFilter.value));
    banners.value = data.list;
    hasMoreBanners.value = data.has_more;
    loadCoverSizes(data.list);
  } catch {
    message.error("轮播列表加载失败");
  } finally {
    loadingBanners.value = false;
  }
}

/** 按封面地址量尺寸；同一地址只发一次，失败结果（null）也缓存。 */
function ensureCoverSize(url?: string | null): Promise<ImageSize | null> {
  if (!url) return Promise.resolve(null);
  if (Object.prototype.hasOwnProperty.call(coverSizes.value, url)) {
    return Promise.resolve(coverSizes.value[url] ?? null);
  }
  const pendingRequest = coverSizeRequests.get(url);
  if (pendingRequest) return pendingRequest;

  const request = readImageSize(url, () => new Image()).then((size) => {
    coverSizes.value[url] = size;
    coverSizeRequests.delete(url);
    // 尺寸是图片解码后才到位的，复制一份 data 让表格重新渲染。
    banners.value = [...banners.value];
    return size;
  });
  coverSizeRequests.set(url, request);
  return request;
}

/** 列表刷新后顺手把每行封面的尺寸排进解码队列（不阻塞列表本身）。 */
function loadCoverSizes(rows: BannerModel[]) {
  const urls = new Set(rows.map((row) => row.cover).filter((url) => Boolean(url)));
  for (const url of urls) {
    void ensureCoverSize(url);
  }
}

async function refreshImages() {
  loadingImages.value = true;
  try {
    const data = await getImages({ page: imagePage.value, limit: IMAGE_PAGE_SIZE });
    images.value = data.list || [];
    // `/images` 是 page/count 分页（后端 `res.OkWithList` → `{list, count}`），没有 has_more。
    // 这里曾经写 `Boolean(data.has_more)`，字段恒为 undefined → 按钮永久禁用、只能看第一页。
    imageTotal.value = data.count ?? 0;
    hasMoreImages.value = hasNextPage(imagePage.value, IMAGE_PAGE_SIZE, imageTotal.value);
  } catch {
    message.error("图片资源加载失败");
  } finally {
    loadingImages.value = false;
  }
}

function removeBanner(row: BannerModel) {
  dialog.warning({
    title: "删除轮播",
    content: row.href || row.title || String(row.id),
    positiveText: "删除",
    negativeText: "取消",
    async onPositiveClick() {
      await deleteBanners([row.id]);
      message.success("已删除");
      await refreshBanners();
    },
  });
}

function removeImage(row: ImageListItem) {
  if (!row.id) return;
  dialog.warning({
    title: "删除图片",
    content: String(row.id),
    positiveText: "删除",
    negativeText: "取消",
    async onPositiveClick() {
      await deleteImages([String(row.id)]);
      message.success("图片已删除");
      await refreshImages();
    },
  });
}

watch(bannerPage, refreshBanners);
watch(bannerShowFilter, () => {
  if (bannerPage.value === 1) {
    void refreshBanners();
    return;
  }
  // 页码 watcher 负责刷新，避免筛选切换时重复请求。
  bannerPage.value = 1;
});
watch(imagePage, refreshImages);
onMounted(() => {
  void refreshBanners();
  void refreshImages();
});
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card" title="轮播运营">
      <template #header-extra>
        <NSpace :size="8" align="center">
          <NSelect
            v-model:value="bannerShowFilter"
            :options="BANNER_SHOW_FILTER_OPTIONS"
            aria-label="展示状态筛选"
            class="w-40" />
          <NButton type="primary" size="small" data-role="open-create-banner" @click="openCreateEditor">
            新增轮播
          </NButton>
        </NSpace>
      </template>

      <NDataTable :columns="bannerColumns" :data="banners" :loading="loadingBanners" :bordered="false" />
      <div class="mt-4 flex justify-end gap-2">
        <NButton :disabled="bannerPage <= 1" @click="bannerPage--">上一页</NButton>
        <NButton :disabled="!hasMoreBanners" @click="bannerPage++">下一页</NButton>
      </div>
    </NCard>

    <NCard class="admin-card" title="图片资源库">
      <NDataTable :columns="imageColumns" :data="images" :loading="loadingImages" :bordered="false" />
      <div class="mt-4 flex justify-end gap-2">
        <NButton :disabled="imagePage <= 1" @click="imagePage--">上一页</NButton>
        <NButton :disabled="!hasMoreImages" @click="imagePage++">下一页</NButton>
      </div>
    </NCard>

    <!-- 新增与编辑是同一张卡：字段、封面流程、校验完全一致，只有提交目标不同。 -->
    <BannerFormModal
      :show="editorVisible"
      :mode="editorMode"
      :banner="editorTarget"
      @update:show="value => (editorVisible = value)"
      @saved="refreshBanners" />
  </NSpace>
</template>
