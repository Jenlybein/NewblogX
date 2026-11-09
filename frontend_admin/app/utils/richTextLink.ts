/**
 * 富文本卡正文里的链接地址收敛。
 *
 * 为什么单独成模块：后台的富文本编辑器原先只把链接换成占位的
 * `https://example.com`，运营没有任何地方能填真实地址，于是正文里每条链接
 * 都指向 example.com。这里把"运营填的这串东西能不能当链接、该收敛成什么"
 * 收成一个纯函数，编辑器与用例共用同一份判据。
 *
 * 允许集合与读者站的白名单**必须一致**（`frontend/app/utils/sanitizeHtml.ts`
 * 的 `RICH_TEXT_ALLOWED_URI`：http(s) / mailto / tel / 站内相对地址）。
 * 两边只要有一处更宽，用户就会看到"保存成功了、链接却在读者站被剥掉"。
 */

/** 允许的协议（大小写不敏感）。与读者站 `RICH_TEXT_ALLOWED_URI` 对齐。 */
const ALLOWED_SCHEMES = /^(?:https?|mailto|tel):/i
/** "看起来带协议"：漏了这一步，`javascript:` 会被当成裸域名去补 https。 */
const ANY_SCHEME = /^[a-z][a-z0-9+.-]*:/i

export type RichTextLinkResult =
  | { ok: true; url: string }
  | { ok: false; error: string }

/**
 * 把运营输入的地址收敛成可以写进 `href` 的形状。
 *
 * 三件事：
 *   1. 剥掉协议白名单之外的（`javascript:` / `data:` / `vbscript:`）——读者站
 *      反正会剥，早点拒绝才不会让人以为设置成功；
 *   2. 站内相对地址（`/article/123`、`#anchor`）原样保留；
 *   3. 裸域名补 `https://`，避免它被当成同级相对路径。
 */
export function normalizeRichTextLink(raw: string): RichTextLinkResult {
  const trimmed = raw.trim()
  if (!trimmed) {
    return { ok: false, error: '请填写链接地址' }
  }

  // 协议相对地址（`//cdn.example.com/a`）不能当站内路径：补上协议头。
  if (trimmed.startsWith('//')) {
    return { ok: true, url: `https:${trimmed}` }
  }
  // 站内相对地址原样保留：它由读者站自己解析，不该被补成外链。
  if (trimmed.startsWith('/') || trimmed.startsWith('#')) {
    return { ok: true, url: trimmed }
  }

  if (ANY_SCHEME.test(trimmed)) {
    if (!ALLOWED_SCHEMES.test(trimmed)) {
      return { ok: false, error: '只支持 http / https / mailto / tel 或站内相对地址' }
    }
    return { ok: true, url: trimmed }
  }

  return { ok: true, url: `https://${trimmed}` }
}
