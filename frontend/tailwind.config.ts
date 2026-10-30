// 读者站 Tailwind 配置（JellyBin 视觉体系）。
//
// 为什么必须有这个文件：以前两端都没有 tailwind.config，Tailwind 走默认
// `darkMode: "media"` —— 站内明暗开关（cookie `blogx-theme` → body 上的
// `dark` 类）**管不住**任何 `dark:` 工具类，那 8 处 `dark:` 只跟操作系统走。
// 这里把策略改成 `class`，与站点根类（见 app/utils/themeClass.ts）对齐。
//
// 颜色：这里**不写任何颜色字面量**，只把语义 token 变量映射成类名。颜色的唯一
// 来源是 app/assets/css/tokens.css（原始层五色相 → 语义层 → 明暗两份映射）。
// 组件消费语义层：bg-page / bg-subtle / bg-surface / text-ink / text-ink-muted /
// border-line / border-line-strong / border-line-faint / bg-highlight /
// bg-danger / bg-danger-soft / border-danger-line / text-danger-ink / ring-focus。
// 原始层 `jelly-*` 只给装饰性记号（三角/方/圆几何点缀）使用。
//
// 排版尺度同样来自 token：max-w-measure（68ch）、leading-prose（1.75）、
// font-title（700 + 负字距由 tracking-title 表达）、font-cjk（中文整段字体栈）。
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
          soft: "var(--color-danger-soft)",
          line: "var(--color-danger-line)",
        },
        focus: "var(--color-focus)",
        shadow: "var(--color-shadow)",
        // 玻璃层填充/边缘（ADR-0012 决策 5；本票只接线，实现留给批次 3）
        glass: {
          thin: "var(--glass-thin-fill)",
          thick: "var(--glass-thick-fill)",
          rim: "var(--glass-rim)",
          "rim-warm": "var(--glass-rim-warm)",
        },
        // 原始层：只给几何记号/装饰性铺色用（文字用 ink / danger-ink，永不用黄）
        jelly: {
          blue: "var(--jelly-blue)",
          cream: "var(--jelly-cream)",
          yellow: "var(--jelly-yellow)",
          coral: "var(--jelly-coral)",
          warm: "var(--jelly-warm)",
        },
      },
      borderRadius: {
        // 圆角阶梯（ADR-0012 决策 4）：控件 / 输入 / 卡片 / 浮层 / 胶囊。
        // 刻意不复用 Tailwind 默认的 sm/md/lg 键名，避免本票顺带把旧圆角改掉。
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
