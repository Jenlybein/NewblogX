// 管理后台 Tailwind 配置（JellyBin 视觉体系，与读者站同源）。
//
// 颜色：这里**不写任何颜色字面量**，只把语义 token 变量映射成类名；颜色的唯一
// 来源是 app/assets/css/tokens.css（两端逐字节同一份，见 scripts/check-token-parity.mjs）。
//
// #22 起后台与读者站同一套中性石墨阶并正式启用暗色：
//   - `darkMode: "class"`：站内开关（cookie `blogx-theme` → 根类 `.dark`）才管得住
//     `dark:` 工具类；走 media 就会变成"用户系统是暗色、后台却被强制变暗"。
//   - 原始层色相 `jelly-*` 不再映射成类名：后台没有"直接用色相"的组件，
//     克莱因蓝退役后也没有它的位置；要新增用法必须先走语义层。
export default {
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        // 语义层（组件唯一消费对象）
        page: "var(--color-bg-page)",
        subtle: "var(--color-bg-subtle)",
        surface: "var(--color-surface)",
        "surface-raised": "var(--color-surface-raised)",
        "surface-inverse": "var(--color-surface-inverse)",
        ink: {
          DEFAULT: "var(--color-ink)",
          muted: "var(--color-ink-muted)",
          faint: "var(--color-ink-faint)",
        },
        // 墨水实心面 / 深墨面（登录 hero、封面兜底）上的反白字
        inverse: {
          DEFAULT: "var(--color-on-inverse)",
          muted: "var(--color-on-inverse-muted)",
        },
        line: {
          DEFAULT: "var(--color-line)",
          strong: "var(--color-line-strong)",
          faint: "var(--color-line-faint)",
        },
        // 功能蓝：只给文字链接；焦点环仍从 focus 取（两者同源，语义不同）。
        link: "var(--color-link)",
        highlight: "var(--color-highlight)",
        danger: {
          DEFAULT: "var(--color-danger)",
          ink: "var(--color-danger-ink)",
          soft: "var(--color-danger-soft)",
          line: "var(--color-danger-line)",
        },
        focus: "var(--color-focus)",
        shadow: "var(--color-shadow)",
        // 玻璃层填充/边缘：后台不上玻璃，但这些类名与读者站共用同一组种子。
        glass: {
          thin: "var(--glass-thin-fill)",
          thick: "var(--glass-thick-fill)",
          rim: "var(--glass-rim)",
          "rim-warm": "var(--glass-rim-warm)",
        },
      },
      borderRadius: {
        // 圆角阶梯（ADR-0012 决策 4）：控件 / 输入 / 卡片 / 浮层 / 胶囊。
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
