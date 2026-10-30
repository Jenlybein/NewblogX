# Apple.com premium aesthetic + Liquid Glass — CSS deconstruction

For plain CSS + custom properties (no AppKit/SwiftUI). ✅ = read out of a real Apple CSS bundle this session; ⚠️ = calibrated target, **not** extracted from Apple CSS (see *What I could not verify*).

## 1. Liquid Glass primary facts

"Liquid Glass is a material that blurs content behind it, reflects color and light of surrounding content, and reacts to touch and pointer interactions in real time." — [Applying Liquid Glass to custom views](https://developer.apple.com/documentation/swiftui/applying-liquid-glass-to-custom-views)

| Fact | Apple's own words | Source |
|---|---|---|
| Definition | "The primary way Liquid Glass visually defines itself is through something called **Lensing**." Earlier materials "scattered light"; this one "dynamically bends, shapes, and concentrates light in real time." | [WWDC25-219](https://developer.apple.com/videos/play/wwdc2025/219/) |
| Motion | "materialize in and out by… modulating the light bending and lensing" — **not** an opacity fade | same |
| Applied to | "a distinct functional layer for **controls and navigation elements**" / "applies to the **topmost layer** of the interface, where you define your navigation" | [Adopting Liquid Glass](https://developer.apple.com/documentation/TechnologyOverviews/adopting-liquid-glass) |
| NOT for content | "Liquid Glass seeks to **bring attention to the underlying content**… overusing this material in custom controls **distracts from that content**." Also: "Reduce your use of custom backgrounds in controls and navigation elements." | same |
| Variants | `.regular` (default), `.clear`, `.identity`; requires iOS/macOS **26.0** | [SwiftUI: Glass](https://developer.apple.com/documentation/swiftui/glass) |
| Contrast rule | "Optimize for legibility when content scrolls beneath controls… a **scroll edge effect**… helps maintain sufficient legibility and contrast for controls by **obscuring content that scrolls beneath them**." | Adopting Liquid Glass |
| Reduce Transparency | "people can… turn on accessibility settings that **reduce transparency or motion**… These settings can **remove or modify certain effects**." | same |
| Performance ceiling | "Applying **too many** effects… can **degrade performance**. Limit the use of Liquid Glass effects onscreen at the same time." | Applying Liquid Glass… |
| Shape system | **fixed** (constant radius) / **capsule** (radius = ½ height) / **concentric** (parent radius − padding); colors retuned for Light/Dark/Increased Contrast | [WWDC25-356](https://developer.apple.com/videos/play/wwdc2025/356/) |

## 2. Perceptual recipe — real numbers

Verbatim from Apple's nav bundle [`ac-globalnav.built.css`](https://www.apple.com/ac/globalnav/7/en_US/styles/ac-globalnav.built.css):

| Token | Value ✅ |
|---|---|
| Font stack | `"SF Pro Text","Myriad Set Pro","SF Pro Icons","Helvetica Neue","Helvetica","Arial",sans-serif` |
| Body | `17px / 1.17648 / letter-spacing -0.022em` |
| Micro | `12px / 1.33337 / -0.01em` |
| Nav bg, no blur | `rgba(0,0,0,0.92)` |
| Nav bg, blur supported | `rgba(0,0,0,0.8)` + `saturate(180%) blur(20px)` |
| Nav dark | `rgba(22,22,23,0.92)` → `rgba(22,22,23,0.8)` |
| Nav height / z-index | `44px` (`48px` ≤833px) / `9999` |
| Column | `max-width: 980px; padding: 0 22px` |
| Radii | `12px` block buttons, `10px`, `8px`, `5px` |
| Hairlines | `#e3e3e3` light / `#424245` dark |
| Text grays / accent | `#1d1d1f`, `#6e6e73`, `#86868b` / `#2997ff`, `#06c` |
| Focus ring | `0 0 0 4px rgba(0,125,250,0.6)` |
| Easings | `cubic-bezier(0.52,0.16,0.24,1)`, `cubic-bezier(0.32,0.08,0.24,1)`; breakpoints `419/833/1023/1044` |

Marketing-layer targets ⚠️: display type `clamp(40px, 7vw, 80px)`, `line-height 1.05–1.07`, `letter-spacing -0.015em…-0.02em`, weight 600–700; section padding `clamp(64px, 9vw, 120px)`; card radii `18px`→`28px`, max two per view; accent ≤5% of painted area.

## 3. CSS implementation

```css
:root{ --glass-blur:20px; --glass-sat:180%; --glass-fill:255 255 255;
        --glass-alpha:.14; --r:18px; }

.glass{
  position:relative; isolation:isolate; border-radius:var(--r);
  background:rgb(var(--glass-fill) / var(--glass-alpha));
  -webkit-backdrop-filter:saturate(var(--glass-sat)) blur(var(--glass-blur));
          backdrop-filter:saturate(var(--glass-sat)) blur(var(--glass-blur));
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / .30),   /* top specular line */
    inset 0 -1px 0 rgb(255 255 255 / .08),  /* bottom bounce */
    0 0 0 1px rgb(255 255 255 / .16),       /* hairline rim */
    0 14px 34px -12px rgb(0 0 0 / .38);     /* lift */
}
/* 1px refracting rim + top arc, painted inside the border box */
.glass::before{
  content:""; position:absolute; inset:0; border-radius:inherit; padding:1px;
  pointer-events:none; opacity:.85;
  background:
    linear-gradient(180deg, rgb(255 255 255/.55), rgb(255 255 255/0) 42%),
    linear-gradient(90deg, rgb(255 255 255/.30), transparent 20%, transparent 80%, rgb(255 255 255/.30));
  -webkit-mask:linear-gradient(#000,#000) content-box, linear-gradient(#000,#000);
          mask:linear-gradient(#000,#000) content-box, linear-gradient(#000,#000);
  -webkit-mask-composite:xor; mask-composite:exclude;
}
.glass::after{ /* grain kills 8-bit banding inside the blur */
  content:""; position:absolute; inset:0; border-radius:inherit; pointer-events:none;
  opacity:.055; mix-blend-mode:overlay;
  background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.8' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='160' height='160' filter='url(%23n)'/%3E%3C/svg%3E");
}
```

Cursor specular — two CSS vars, no layout reads in the paint path:

```js
el.addEventListener('pointermove', e => {
  const r = el.getBoundingClientRect();
  el.style.setProperty('--mx', `${(e.clientX-r.left)/r.width*100}%`);
  el.style.setProperty('--my', `${(e.clientY-r.top)/r.height*100}%`);
});
```
```css
.glass::before{ background:
  radial-gradient(220px circle at var(--mx,50%) var(--my,0%),
    rgb(255 255 255/.45), rgb(255 255 255/0) 60%), /* + arc gradients */ ; }
```

```css
@supports not ((backdrop-filter:blur(1px)) or (-webkit-backdrop-filter:blur(1px))){
  .glass{ background:rgb(28 28 30 / .92); }
}
@media (prefers-reduced-transparency: reduce){   /* Safari; not Baseline */
  .glass{ background:rgb(28 28 30 / .96); -webkit-backdrop-filter:none; backdrop-filter:none; }
}
@media (prefers-reduced-motion: reduce){
  [data-parallax]{ transform:none !important; }
  .reveal{ opacity:1; transform:none; transition:none; }
}
```

**Performance.** `backdrop-filter` is [Baseline 2024](https://developer.mozilla.org/en-US/docs/Web/CSS/backdrop-filter); `prefers-reduced-transparency` is ["not Baseline"](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@media/prefers-reduced-transparency). Any element with `backdrop-filter`, `filter`, `opacity<1`, `mask`, `clip-path`, `mix-blend-mode` or `will-change` on those becomes a **backdrop root** — a glass card *inside* a glass panel can't see the page, it reads flat grey. Each blurred layer is a GPU pass: budget one, maybe two. Never animate `blur()`; animate `transform`/`opacity`. Mobile Safari needs `-webkit-` and historically drops the filter on transformed ancestors — keep `transform` off the glass element.

## 4. Dark mode

Site near-black is `#1d1d1f`, not `#000` (✅). iOS *system* blacks split: `#000` (systemBackground, OLED pixels off) vs `#1C1C1E` / `#2C2C2E` for elevated layers. Trap: white fill at `.14` over `#000` yields a flat grey slab — the fill contributes nothing, so **the rim must carry the read**: drop fill to `.06–.10`, raise rim to `rgb(255 255 255 / .28–.34)`. `saturate(180%)` is a no-op over neutral black; skip `brightness(>1)`, it greys the black out. For text on glass use Apple's approach — a scroll-edge scrim `linear-gradient(to bottom, rgb(0 0 0/.55), transparent)` — rather than more fill.

## 5. Anti-patterns

| Smell | Why it's cheap | Fix |
|---|---|---|
| `saturate(200%+)`, `blur(40px+)` | Neon smear; Apple ships `180% / 20px` | cap at 180%/20px |
| Cyan/teal tint | Reads as 2010 frosted knockoff | neutral fill; brand tint ≤6% |
| 3+ blurred layers | Backdrop roots isolate → grey + N passes | one glass layer per viewport |
| Glass behind body copy | Contrast dies, text aliases | glass on controls/overlays only |
| 12–14px text on `.14` alpha | Fails contrast over media | ≥16px, or fill `.45`+ |
| Gradient soup, 3 hue shifts | No single light source | one direction, one arc |
| `box-shadow: 0 0 40px accent` | Glass edges don't bloom | neutral black, large + soft |
| Animating blur | Per-frame backdrop repaint | animate transform/opacity |

## What I could not verify

- **Product-page CSS is unreachable.** Bundles are build-hashed (`/v/<product>/<build>/…`); every guessed path 404'd and page HTML truncated before `<head>`. Display sizes, padding, radii and grain values above are **targets**; §2 ✅ rows are the only verbatim Apple CSS.
- **HIG "Materials" prose is unreachable.** `/design/human-interface-guidelines/materials` and its JSON endpoints 404/return a JS shell here, so regular-vs-clear *usage rules* come from the SwiftUI `Glass` API and WWDC transcripts, not HIG prose.
- Apple publishes **no** numeric contrast threshold for Liquid Glass; 4.5:1 is my WCAG-derived expectation.

## Sources

- [Adopting Liquid Glass](https://developer.apple.com/documentation/TechnologyOverviews/adopting-liquid-glass)
- [SwiftUI: Applying Liquid Glass to custom views](https://developer.apple.com/documentation/swiftui/applying-liquid-glass-to-custom-views)
- [SwiftUI: Glass](https://developer.apple.com/documentation/swiftui/glass)
- [WWDC25-219 Meet Liquid Glass](https://developer.apple.com/videos/play/wwdc2025/219/)
- [WWDC25-356 Get to know the new design system](https://developer.apple.com/videos/play/wwdc2025/356/)
- [apple.com global nav CSS (verbatim)](https://www.apple.com/ac/globalnav/7/en_US/styles/ac-globalnav.built.css)
- [MDN: backdrop-filter](https://developer.mozilla.org/en-US/docs/Web/CSS/backdrop-filter)
- [MDN: prefers-reduced-transparency](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@media/prefers-reduced-transparency)
