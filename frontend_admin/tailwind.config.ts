// 管理后台 Tailwind 配置（JellyBin 视觉体系，与读者站同源）。
//
// 两端此前都没有 tailwind.config，Tailwind 走默认 `darkMode: "media"`。后台虽然
// 没有明暗开关（spec 明说后台暗色不做），但策略仍显式设成 `class`：
//   1. 与读者站保持同一口径，避免"以后给后台加开关时又要改一处"；
//   2. media 策略下 `dark:` 工具类会跟操作系统走，将来后台接暗色会立刻出现
//      "用户系统是暗色、后台却被强制变暗"的事故。
//
// 颜色：这里**不写任何颜色字面量**，只把语义 token 变量映射成类名；颜色的唯一
// 来源是 app/assets/css/tokens.css。后台组件消费语义层，克制使用（不做暗色切换、
// 不做捏捏、动效只到 hover/focus）。
export default {
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        page: "var(--color-bg-page)",
        subtle: "var(--color-bg-subtle)",
        surface: "var(--color-surface)",
        "surface-raised": "var(--color-surface-raised)",
        ink: {
          DEFAULT: "var(--color-ink)",
          muted: "var(--color-ink-muted)",
        },
        line: {
          DEFAULT: "var(--color-line)",
          strong: "var(--color-line-strong)",
          faint: "var(--color-line-faint)",
        },
        highlight: "var(--color-highlight)",
        danger: {
          DEFAULT: "var(--color-danger)",
          ink: "var(--color-danger-ink)",
        },
        focus: "var(--color-focus)",
        shadow: "var(--color-shadow)",
        glass: {
          thin: "var(--glass-thin-fill)",
          thick: "var(--glass-thick-fill)",
          rim: "var(--glass-rim)",
          "rim-warm": "var(--glass-rim-warm)",
        },
        jelly: {
          blue: "var(--jelly-blue)",
          cream: "var(--jelly-cream)",
          yellow: "var(--jelly-yellow)",
          coral: "var(--jelly-coral)",
          warm: "var(--jelly-warm)",
        },
      },
      borderRadius: {
        // 圆角阶梯，键名不复用默认 sm/md/lg（本票只播种，不改既有圆角观感）。
        control: "var(--radius-sm)",
        field: "var(--radius-md)",
        card: "var(--radius-lg)",
        overlay: "var(--radius-xl)",
        pill: "var(--radius-pill)",
      },
      backdropBlur: {
        "glass-thin": "var(--glass-thin-blur)",
        "glass-thick": "var(--glass-thick-blur)",
        "glass-mobile": "var(--glass-blur-mobile)",
      },
      backdropSaturate: {
        "glass-thin": "var(--glass-thin-saturate)",
        "glass-thick": "var(--glass-thick-saturate)",
      },
      opacity: {
        grain: "var(--glass-grain-opacity)",
      },
      fontFamily: {
        body: "var(--font-sans)",
        latin: "var(--font-latin)",
        cjk: "var(--font-cjk)",
        display: "var(--font-display)",
        mono: "var(--font-mono)",
        title: "var(--font-title)",
      },
      fontWeight: {
        title: "var(--type-weight-title)",
      },
      lineHeight: {
        prose: "var(--type-leading-prose)",
        title: "var(--type-leading-title)",
      },
      letterSpacing: {
        title: "var(--type-tracking-title)",
        dense: "var(--type-tracking-dense)",
      },
      maxWidth: {
        measure: "var(--type-measure)",
      },
    },
  },
};
