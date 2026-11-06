import { describe, expectTypeOf, it } from "vitest";

import type {
  AIRuntimeConfig,
  DecoRuntimeConfig,
  ImageGCRuntimeConfig,
  LogRuntimeConfig,
  MailRuntimeConfig,
  OAuthRuntimeConfig,
  SiteLoginRuntimeConfig,
  SiteRuntimeConfig,
  StorageRuntimeConfig,
  UploadRuntimeConfig,
} from "~/types/api";
import type { RequiredSchema } from "~/types/contract";

/**
 * 后台「站点」配置的编译期契约对账（#60 / #192 / #202 / ADR-0017 / ADR-0026；#283 改写）。
 *
 * #283 前这一层是「逐字段解析 `app/types/api.ts` 里手写视图模型 + .vue 模板字符串」的
 * 源码文本断言。九段具名进契约（开放接口契约）之后，手写视图模型整体退场、两端类型都从
 * `openapi.d.ts` 派生，字段名与类型由生成产物在编译期给出；本文件随之改成**纯契约类型
 * 派生断言**：只 `import type` 生成的具名段 schema，用 `expectTypeOf` 证明形状来自契约，
 * 不读源码文本、不复制字段清单、不新增源码文本断言。
 *
 * 原本 13 条用例的语义事实全部保留，只是判定口径从「源码里有没有某个键/某句文案」
 * 换成「契约形状是否如此」：
 *   1. 站点模式 / 站点标题 / Logo / 旧右栏键都不在契约里 → 写回这些键过不了类型检查；
 *   2. 发布与展示三开关、私信限流与陌生人开关都是具名 `Article` / `Chat` 形状的强类型字段；
 *   3. 九段各有具名响应形状，后台类型一一派生自它们。
 *
 * `expectTypeOf` 在运行时是 no-op，真正的判定发生在 `nuxi typecheck`；这与读者站
 * `test/gates/site-runtime-config-parity.test.ts` 同一套做法。
 */
type SiteSectionResponse = RequiredSchema<"internal_site_adapter_http.SiteSectionSiteResponse">;
type SiteInfoShape = RequiredSchema<"myblogx_conf_site.SiteInfo">;
type ArticleShape = RequiredSchema<"myblogx_conf_site.Article">;
type ChatShape = RequiredSchema<"myblogx_conf_site.Chat">;
type LoginShape = RequiredSchema<"myblogx_conf_site.Login">;

describe("「站点模式」整条链已删除（#60）", () => {
  it("site 段就是具名契约形状，且 site_info 里没有 mode", () => {
    // SiteRuntimeConfig 从 openapi.d.ts 派生，不再是手写视图模型。
    expectTypeOf<SiteRuntimeConfig>().toEqualTypeOf<SiteSectionResponse>();
    expectTypeOf<keyof SiteInfoShape>().toEqualTypeOf<"beian">();
    expectTypeOf<"mode" extends keyof SiteInfoShape ? true : false>().toEqualTypeOf<false>();
    expectTypeOf<"mode" extends keyof SiteSectionResponse ? true : false>().toEqualTypeOf<false>();
  });

  it("后台类型就是 site 段具名响应（不是第三份字段清单）", () => {
    expectTypeOf<SiteRuntimeConfig["site_info"]>().toEqualTypeOf<SiteInfoShape>();
    expectTypeOf<SiteRuntimeConfig["article"]>().toEqualTypeOf<ArticleShape>();
    expectTypeOf<SiteRuntimeConfig["chat"]>().toEqualTypeOf<ChatShape>();
    expectTypeOf<SiteRuntimeConfig["login"]>().toEqualTypeOf<LoginShape>();
  });
});

describe("旧右栏配置已从后台视图模型下线", () => {
  it("site 段具名契约不再有 index_right / site_notice / hot_tags / recommended_authors", () => {
    // 契约里没有这个键：任何想把废弃键写回 site 段的代码都过不了类型检查。
    expectTypeOf<"index_right" extends keyof SiteSectionResponse ? true : false>().toEqualTypeOf<false>();
    expectTypeOf<"site_notice" extends keyof SiteSectionResponse ? true : false>().toEqualTypeOf<false>();
    expectTypeOf<"hot_tags" extends keyof SiteSectionResponse ? true : false>().toEqualTypeOf<false>();
    expectTypeOf<"recommended_authors" extends keyof SiteSectionResponse ? true : false>().toEqualTypeOf<false>();
  });

  it("九段各有具名响应形状，后台类型一一派生自它们", () => {
    expectTypeOf<AIRuntimeConfig>().toEqualTypeOf<
      RequiredSchema<"internal_site_adapter_http.SiteSectionAiResponse">
    >();
    expectTypeOf<MailRuntimeConfig>().toEqualTypeOf<
      RequiredSchema<"internal_site_adapter_http.SiteSectionMailResponse">
    >();
    expectTypeOf<StorageRuntimeConfig>().toEqualTypeOf<
      RequiredSchema<"internal_site_adapter_http.SiteSectionStorageResponse">
    >();
    expectTypeOf<OAuthRuntimeConfig>().toEqualTypeOf<
      RequiredSchema<"internal_site_adapter_http.SiteSectionOauthResponse">
    >();
    expectTypeOf<LogRuntimeConfig>().toEqualTypeOf<
      RequiredSchema<"internal_site_adapter_http.SiteSectionLogResponse">
    >();
    expectTypeOf<UploadRuntimeConfig>().toEqualTypeOf<
      RequiredSchema<"internal_site_adapter_http.SiteSectionUploadResponse">
    >();
    expectTypeOf<DecoRuntimeConfig>().toEqualTypeOf<
      RequiredSchema<"internal_site_adapter_http.SiteSectionDecoResponse">
    >();
    expectTypeOf<ImageGCRuntimeConfig>().toEqualTypeOf<
      RequiredSchema<"internal_site_adapter_http.SiteSectionImageGcResponse">
    >();
  });
});

describe("站点标题与 Logo 自定义已删除（#192）", () => {
  it("基础信息子块只有备案号，没有标题与 Logo", () => {
    expectTypeOf<SiteInfoShape>().toEqualTypeOf<{ beian: string }>();
    expectTypeOf<"title" extends keyof SiteInfoShape ? true : false>().toEqualTypeOf<false>();
    expectTypeOf<"logo" extends keyof SiteInfoShape ? true : false>().toEqualTypeOf<false>();
  });

  it("site_info 的备案号是字符串（保存载荷不再带被删字段）", () => {
    expectTypeOf<SiteInfoShape["beian"]>().toEqualTypeOf<string>();
    expectTypeOf<SiteRuntimeConfig["site_info"]["beian"]>().toEqualTypeOf<string>();
  });
});

describe("「发布与展示」三开关（#60 / ADR-0017 决策 2）", () => {
  it("article 具名形状声明三个布尔", () => {
    expectTypeOf<ArticleShape["allow_non_admin_publish"]>().toEqualTypeOf<boolean>();
    expectTypeOf<ArticleShape["allow_non_admin_draft"]>().toEqualTypeOf<boolean>();
    expectTypeOf<ArticleShape["show_non_admin_articles"]>().toEqualTypeOf<boolean>();
  });

  it("site 段的 article 子块就是具名 Article 形状", () => {
    expectTypeOf<SiteRuntimeConfig["article"]>().toEqualTypeOf<ArticleShape>();
    expectTypeOf<SiteRuntimeConfig["article"]["allow_non_admin_publish"]>().toEqualTypeOf<boolean>();
  });

  it("登录策略子块也是具名 Login 形状（不是独立视图模型）", () => {
    expectTypeOf<SiteLoginRuntimeConfig>().toEqualTypeOf<LoginShape>();
    expectTypeOf<SiteRuntimeConfig["login"]>().toEqualTypeOf<SiteLoginRuntimeConfig>();
  });
});

describe("私信的陌生人开关（#202）", () => {
  it("chat 具名形状声明陌生人开关布尔", () => {
    expectTypeOf<ChatShape["stranger_msg_enabled"]>().toEqualTypeOf<boolean>();
    expectTypeOf<SiteRuntimeConfig["chat"]["stranger_msg_enabled"]>().toEqualTypeOf<boolean>();
  });

  it("私信限流参数在具名契约里同名且是数值（ADR-0026）", () => {
    expectTypeOf<ChatShape["rate_window_sec"]>().toEqualTypeOf<number>();
    expectTypeOf<ChatShape["rate_daily_max"]>().toEqualTypeOf<number>();
    expectTypeOf<SiteRuntimeConfig["chat"]["rate_window_sec"]>().toEqualTypeOf<number>();
    expectTypeOf<SiteRuntimeConfig["chat"]["rate_daily_max"]>().toEqualTypeOf<number>();
  });

  it("私信开关是站点级策略：chat 子块属于 site 段具名形状", () => {
    // 站点级 = 它随整段 site 一起读写，不在用户行上（零 DDL 不给既有表加列）。
    expectTypeOf<SiteRuntimeConfig["chat"]>().toEqualTypeOf<ChatShape>();
    expectTypeOf<"chat" extends keyof SiteSectionResponse ? true : false>().toEqualTypeOf<true>();
  });
});
