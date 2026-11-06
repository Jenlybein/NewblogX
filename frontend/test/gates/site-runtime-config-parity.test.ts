import { describe, expectTypeOf, it } from "vitest";

import type { SiteDecoConfig, SiteRuntimeConfig } from "~/types/api";
import type { RequiredSchema } from "~/types/contract";

/**
 * 九段具名契约派生类型的编译期对账（#283）。
 *
 * 旧版逐字段解析 `backend/conf/site/enter.go` 的 Go json tag 与 `app/types/api.ts`
 * 的 TS 键，是源码文本断言；九段响应具名进契约（开放接口契约）之后，字段名与
 * 类型都由生成产物给出，手写视图模型与那套解析门禁一并退场。
 *
 * 这里只做类型层断言：消费的是两端从 swagger.json 各自派生的 openapi.d.ts，
 * 不读任何源码文本，也不复制一份字段清单。
 */
describe("运行时配置类型来自九段具名契约", () => {
  it("读者站站点配置就是 site 段的具名响应形状", () => {
    expectTypeOf<SiteRuntimeConfig>().toEqualTypeOf<
      RequiredSchema<"internal_site_adapter_http.SiteSectionSiteResponse">
    >();
  });

  it("发布/展示开关与备案号由具名形状给出", () => {
    expectTypeOf<SiteRuntimeConfig["site_info"]["beian"]>().toEqualTypeOf<string>();
    expectTypeOf<
      SiteRuntimeConfig["article"]["allow_non_admin_publish"]
    >().toEqualTypeOf<boolean>();
    expectTypeOf<
      SiteRuntimeConfig["article"]["allow_non_admin_draft"]
    >().toEqualTypeOf<boolean>();
    expectTypeOf<
      SiteRuntimeConfig["article"]["show_non_admin_articles"]
    >().toEqualTypeOf<boolean>();
  });

  it("装饰段的数值键允许 null（归一化层据此回落默认值）", () => {
    expectTypeOf<SiteDecoConfig["charm"]>().toMatchTypeOf<
      { charm?: { logo_size?: number | null } | null }
    >();
  });
});
