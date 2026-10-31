import { describe, expect, it } from "vitest";

import type { SiteRuntimeConfig } from "~/types/api";
import {
  DRAFT_DISABLED_NOTICE,
  PUBLISH_DISABLED_NOTICE,
  articleWritePolicy,
  editorReadOnly,
  writeEntryVisible,
  writePolicyNotices,
} from "~/utils/writePolicy";

/**
 * 「发布与展示」两个写作开关的读者站投影（#64）。
 *
 * 期望值来自 ADR-0017 决策 2 的前端部分：
 *   - 发布开关关闭 → 隐藏「写文章」入口；
 *   - 草稿开关关闭 → 编辑器只读，保存/发布禁用并说明原因；
 *   - 两个开关都开（或配置还没取回）→ 什么都不变。
 *
 * 服务端判定仍是唯一边界，这里只是体验层：因此"配置缺席"必须回落到全开，
 * 不能因为一次请求失败就把全站作者锁成只读。
 */

function configWith(article: SiteRuntimeConfig["article"]): SiteRuntimeConfig {
  return {
    site_info: { title: "果冻盒" },
    project: { title: "JellyBin" },
    login: { username_pwd_login: true },
    index_right: { list: [] },
    article,
  };
}

describe("articleWritePolicy", () => {
  it("站点配置还没取回时按默认值全开", () => {
    expect(articleWritePolicy(null)).toEqual({ allowPublish: true, allowDraft: true });
    expect(articleWritePolicy(undefined)).toEqual({ allowPublish: true, allowDraft: true });
  });

  it("老后台发出的配置里没有 article 段时同样全开", () => {
    const config = configWith({});
    expect(articleWritePolicy(config)).toEqual({ allowPublish: true, allowDraft: true });
  });

  it("显式关闭的开关被如实读出（false 不能被真值兜底吞掉）", () => {
    expect(articleWritePolicy(configWith({ allow_non_admin_publish: false }))).toEqual({
      allowPublish: false,
      allowDraft: true,
    });
    expect(articleWritePolicy(configWith({ allow_non_admin_draft: false }))).toEqual({
      allowPublish: true,
      allowDraft: false,
    });
    expect(
      articleWritePolicy(
        configWith({ allow_non_admin_publish: false, allow_non_admin_draft: false }),
      ),
    ).toEqual({ allowPublish: false, allowDraft: false });
  });

  it("展示开关（show_non_admin_articles）不参与写作判定", () => {
    expect(articleWritePolicy(configWith({ show_non_admin_articles: false }))).toEqual({
      allowPublish: true,
      allowDraft: true,
    });
  });
});

describe("writeEntryVisible", () => {
  it("发布开关打开时展示「写文章」入口", () => {
    expect(writeEntryVisible({ allowPublish: true, allowDraft: true })).toBe(true);
    expect(writeEntryVisible({ allowPublish: true, allowDraft: false })).toBe(true);
  });

  it("发布开关关闭时隐藏入口（即使还能存草稿）", () => {
    expect(writeEntryVisible({ allowPublish: false, allowDraft: true })).toBe(false);
    expect(writeEntryVisible({ allowPublish: false, allowDraft: false })).toBe(false);
  });
});

describe("editorReadOnly", () => {
  it("草稿开关打开时编辑器可写", () => {
    expect(editorReadOnly({ allowPublish: true, allowDraft: true })).toBe(false);
    expect(editorReadOnly({ allowPublish: false, allowDraft: true })).toBe(false);
  });

  it("草稿开关关闭时编辑器只读", () => {
    expect(editorReadOnly({ allowPublish: true, allowDraft: false })).toBe(true);
    expect(editorReadOnly({ allowPublish: false, allowDraft: false })).toBe(true);
  });
});

describe("writePolicyNotices", () => {
  it("两个开关都开时没有任何说明", () => {
    expect(writePolicyNotices({ allowPublish: true, allowDraft: true })).toEqual([]);
  });

  it("发布关闭时给出「站点策略」说明，而不是账号问题", () => {
    const notices = writePolicyNotices({ allowPublish: false, allowDraft: true });

    expect(notices).toContain(PUBLISH_DISABLED_NOTICE);
    expect(PUBLISH_DISABLED_NOTICE).toContain("站点策略");
  });

  it("草稿关闭时说明编辑器只读的原因", () => {
    const notices = writePolicyNotices({ allowPublish: true, allowDraft: false });

    expect(notices).toContain(DRAFT_DISABLED_NOTICE);
    expect(DRAFT_DISABLED_NOTICE).toContain("只读");
  });

  it("两个都关时两条说明都在，且发布在前（先解释入口为什么没了）", () => {
    expect(writePolicyNotices({ allowPublish: false, allowDraft: false })).toEqual([
      PUBLISH_DISABLED_NOTICE,
      DRAFT_DISABLED_NOTICE,
    ]);
  });
});
