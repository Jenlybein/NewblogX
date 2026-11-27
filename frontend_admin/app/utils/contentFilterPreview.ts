/**
 * 关键词过滤的「本地预览」（纯函数，不发请求）。
 *
 * 为什么要有它：重排后的词表页要能当场回答"这段文字发出去会变成什么样"，
 * 白名单页也要用「输入 → 发布后」两列讲清楚两种白名单粒度。这些演示如果都走后端，
 * 每敲一个字就要发一次请求（而且没有这样的匿名端点）；在后端也不太合适 ——
 * 预览只用于配置时自检，不该多出一条公开接口。
 *
 * 口径对齐后端 `contentfilter` 引擎的三条关键规则（只对齐到"演示够用"的程度，
 * 不重写整个归一化/扫描器 —— 真值仍然后端说了算）：
 *
 *   1. 匹配前归一化：全角折半角、拉丁转小写、零宽字符与空白/标点被压掉。
 *      所以「1氨丙酮」「１氨丙酮」「1 氨 丙酮」在演示里是同一个词；
 *   2. **两档都替换**：命中后替换成等长 `*`（档位只决定风险页的颜色，不影响文本）；
 *   3. 白名单是「区间覆盖」语义：词级白名单让该词的所有命中都不打星；
 *      短语白名单只让**被短语完整包住**的那次命中不打星，短语外面的同名词照旧打星。
 *
 * 输出同时给"整段打星结果"和"分段（是否被打星）"：模板要按段渲染 `<mark>`，
 * 而单测只需要断言 `masked` 与 `hits` 两样。
 */

/** 演示里要打星的一段文字（`masked=true` 的段会被渲染成 <mark>）。 */
export interface ContentFilterPreviewSegment {
  text: string
  masked: boolean
}

export interface ContentFilterPreviewResult {
  /** 打星后的整段文字（长度与入参一致，一个原字符对应一个 `*`）。 */
  masked: string
  /** 命中处数：被真正替换掉的词出现次数（白名单覆盖的不算）。 */
  hits: number
  segments: ContentFilterPreviewSegment[]
}

/** 词表里参与预览的形状：只要有 `word` 就够（真实词条还带档位/类目）。 */
export interface ContentFilterPreviewWord {
  word: string
}

/**
 * 归一化时视作"分隔符"的字符：空白、零宽字符与常见中日韩/英文标点。
 *
 * 注意：这里**不把字母数字之外的符号都删掉**，只删这些明确的分隔符 ——
 * 演示要尽量贴近引擎的"压掉空白与标点"，而不是把所有符号都吞了（否则
 * 「C++」这类词条自己都会被归一化没掉）。
 */
const SEPARATOR_RE =
  /[\s\u200b-\u200f\u2060\ufeff·、。，．,.；;：:！!？?…—－\-_/\\|"'“”‘’()（）[\]【】《》<>{}]/u

/** 把全角 ASCII（U+FF01–U+FF5E）折成半角；全角空格折成普通空格。 */
function foldFullWidth(char: string): string {
  const code = char.codePointAt(0) ?? 0
  if (code >= 0xff01 && code <= 0xff5e) {
    return String.fromCodePoint(code - 0xfee0)
  }
  if (code === 0x3000) return " "
  return char
}

interface NormalizedText {
  /** 归一化后的可匹配文本（分隔符已被压掉）。 */
  text: string
  /** `map[i]` = 归一化文本第 i 个字符来自原文的第几个字符（按码点计）。 */
  map: number[]
}

function normalizeForMatch(input: string): NormalizedText {
  const chars = Array.from(input)
  const out: string[] = []
  const map: number[] = []
  for (const [index, char] of chars.entries()) {
    const folded = foldFullWidth(char).toLowerCase()
    // 分隔符整体跳过：它既不出现在可匹配文本里，也不占 map —— 于是
    // 「1 氨 丙酮」的命中区间在原文里会自动跨过被压掉的那个空格。
    if (folded.length === 0 || SEPARATOR_RE.test(folded)) continue
    out.push(folded)
    map.push(index)
  }
  return { text: out.join(""), map }
}

/** 在 haystack 里找 needle 的全部出现位置（允许重叠，和引擎的逐位置扫描同口径）。 */
function findOccurrences(haystack: string, needle: string): number[] {
  if (!needle) return []
  const found: number[] = []
  let from = 0
  for (;;) {
    const at = haystack.indexOf(needle, from)
    if (at < 0) return found
    found.push(at)
    from = at + 1
  }
}

/**
 * 把一段文字按当前词表与两份白名单打星。
 *
 * @param text 要演示的原文
 * @param words 词表（有 `word` 字段即可）
 * @param whitelist 词级白名单（该词整词豁免）
 * @param whitelistPhrases 短语白名单（被短语完整包住的那次命中豁免）
 */
export function maskContentFilterText(
  text: string,
  words: readonly (ContentFilterPreviewWord | string)[],
  whitelist: readonly string[] = [],
  whitelistPhrases: readonly string[] = [],
): ContentFilterPreviewResult {
  const sourceChars = Array.from(text)
  if (sourceChars.length === 0) {
    return { masked: "", hits: 0, segments: [] }
  }

  const normalized = normalizeForMatch(text)
  const wordWhitelist = new Set(
    whitelist
      .map((item) => normalizeForMatch(item).text)
      .filter((item) => item.length > 0),
  )

  // 短语白名单的覆盖范围先算出来：一次命中只要整段都落在某个短语里就豁免。
  const phraseCovered = new Array<boolean>(normalized.text.length).fill(false)
  for (const phrase of whitelistPhrases) {
    const needle = normalizeForMatch(phrase).text
    if (!needle) continue
    for (const at of findOccurrences(normalized.text, needle)) {
      for (let i = at; i < at + needle.length; i += 1) phraseCovered[i] = true
    }
  }

  const masked = new Array<boolean>(sourceChars.length).fill(false)
  let hits = 0
  const seen = new Set<string>()
  for (const entry of words) {
    const raw = typeof entry === "string" ? entry : entry.word
    const needle = normalizeForMatch(raw).text
    // 同一个词在表里出现两次只演示一次；单字词与后端同口径不入表，这里也不演示。
    if (needle.length < 2 || seen.has(needle)) continue
    seen.add(needle)
    if (wordWhitelist.has(needle)) continue

    for (const at of findOccurrences(normalized.text, needle)) {
      const end = at + needle.length
      let covered = true
      for (let i = at; i < end; i += 1) {
        if (!phraseCovered[i]) {
          covered = false
          break
        }
      }
      if (covered) continue

      // 把归一化区间映射回原文区间：被压掉的分隔符也一起打星（引擎替换的是
      // 整段原文，所以「1 氨 丙酮」会变成 7 个 *，而不是 5 个）。
      const from = normalized.map[at] ?? 0
      const to = normalized.map[end - 1] ?? from
      for (let i = from; i <= to; i += 1) masked[i] = true
      hits += 1
    }
  }

  const segments: ContentFilterPreviewSegment[] = []
  for (const [index, char] of sourceChars.entries()) {
    const flag = masked[index] ?? false
    if (flag) {
      sourceChars[index] = "*"
    }
    const last = segments[segments.length - 1]
    if (last && last.masked === flag) last.text += flag ? "*" : char
    else segments.push({ text: flag ? "*" : char, masked: flag })
  }

  return { masked: sourceChars.join(""), hits, segments }
}
