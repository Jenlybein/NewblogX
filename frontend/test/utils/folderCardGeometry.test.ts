import { describe, expect, it } from "vitest";

import {
  FOLDER_CARD_BASE_HEIGHT,
  FOLDER_CARD_BASE_WIDTH,
  FOLDER_FLAP_HEIGHT,
  FOLDER_FLAP_PATH,
  FOLDER_FLAP_RADIUS,
  FOLDER_FLAP_SHOULDER_Y,
  FOLDER_FLAP_WIDTH,
  folderCardFlapTiltDeg,
  folderCardLiftPx,
  folderCardScale,
  folderCardSheetHeightPx,
  folderCardSheetTiltDeg,
  folderCardStyleVars,
  folderFlapMaskImage,
  folderFlapShoulderY,
  folderSheetTopPx,
} from "~/utils/folderCardGeometry";

/**
 * 文件夹卡的纯几何（grill 记录 §三 3.6）。
 *
 * 为什么这些数字要抽成纯函数：卡片的几何是**比例**（缩放 = 列宽 / 321），
 * 靠肉眼看样式表只能验"某一种列宽下没坏"。把基宽、凸舌肩线、抽卡距离、倾角与
 * 百分比换算收成纯函数后，底座上的每一处比例都能用算式钉住，样式表只消费结果。
 *
 * 外形基准取参考组件的 321×270 舞台 + 321×241 前盖：卡里只有**一张** 16:9 封面
 * （不是三张扇形），封面常态露出上 2/3，肩线以下压在前盖后面。
 */
describe("缩放系数：容器宽度 / 基宽 321", () => {
  it("基宽本身是 1，两倍宽是 2，半宽是 0.5", () => {
    expect(FOLDER_CARD_BASE_WIDTH).toBe(321);
    expect(FOLDER_CARD_BASE_HEIGHT).toBe(270);

    expect(folderCardScale(321)).toBe(1);
    expect(folderCardScale(642)).toBe(2);
    expect(folderCardScale(160.5)).toBe(0.5);
  });

  it("无法测量的宽度按 0 处理（不抛出、不产生 NaN 尺寸）", () => {
    expect(folderCardScale(0)).toBe(0);
    expect(folderCardScale(-100)).toBe(0);
    expect(folderCardScale(Number.NaN)).toBe(0);
    expect(folderCardScale(Number.POSITIVE_INFINITY)).toBe(0);
  });
});

/**
 * 前盖轮廓是「像文件夹」的唯一来源：带凸舌的路径。这些断言钉住它**逐字**没被
 * 手改过——路径里任何一段被"顺手调一下"，凸舌、斜切肩、底部圆角就都不一样了。
 */
describe("前盖轮廓：参考组件的路径逐字保留", () => {
  it("路径与参考完全一致（左上圆角 / 凸舌 / 斜切肩 / 右侧平顶 / 底部圆角）", () => {
    expect(FOLDER_FLAP_PATH).toBe(
      "M0 25C0 11.1929 11.1929 0 25 0H136.084C143.044 0 149.689 2.90139 154.42 8.00608L178.08 33.5343C182.811 38.639 189.456 41.5404 196.416 41.5404H296C309.807 41.5404 321 52.7333 321 66.5404V216C321 229.807 309.807 241 296 241H25C11.1929 241 0 229.807 0 216V25Z",
    );
  });

  it("前盖画布 321×241、圆角 25、肩线 41.5404（路径自己的坐标系）", () => {
    expect(FOLDER_FLAP_WIDTH).toBe(321);
    expect(FOLDER_FLAP_HEIGHT).toBe(241);
    expect(FOLDER_FLAP_RADIUS).toBe(25);
    expect(FOLDER_FLAP_SHOULDER_Y).toBe(41.5404);
  });

  it("凸舌只占左侧（约 61%），右侧从肩线开始平顶 —— 就是「文件夹」那一步台阶", () => {
    expect(FOLDER_FLAP_SHOULDER_Y / FOLDER_FLAP_HEIGHT).toBeCloseTo(0.1724, 3);
    expect(FOLDER_FLAP_SHOULDER_Y).toBeLessThan(FOLDER_FLAP_HEIGHT / 4);
  });
});

describe("封面的 16:9：高度由宽度决定", () => {
  it("210 宽的封面高 118.125（16:9）", () => {
    expect(folderCardSheetHeightPx(210)).toBeCloseTo(118.125, 5);
    expect(folderCardSheetHeightPx(420)).toBeCloseTo(236.25, 5);
  });

  it("非法宽度返回 0", () => {
    expect(folderCardSheetHeightPx(0)).toBe(0);
    expect(folderCardSheetHeightPx(Number.NaN)).toBe(0);
  });
});

describe("抽卡距离：基宽下上抽 30px，等比缩放", () => {
  it("基宽 30px、两倍宽 60px", () => {
    expect(folderCardLiftPx(321)).toBe(30);
    expect(folderCardLiftPx(642)).toBe(60);
  });

  it("宽度测不出来时归零（不产生 NaN 位移）", () => {
    // 「未展开 / 减少动态时归零」不在这里：那是 CSS 的 :hover 与
    // prefers-reduced-motion 兜底的事，算式只回答「展开时抽多远」。
    expect(folderCardLiftPx(0)).toBe(0);
    expect(folderCardLiftPx(-100)).toBe(0);
    expect(folderCardLiftPx(Number.NaN)).toBe(0);
  });
});

describe("倾角：前盖常态 -15°、展开 -45°；封面展开旋转 6°", () => {
  it("前盖两态", () => {
    expect(folderCardFlapTiltDeg(false)).toBe(-15);
    expect(folderCardFlapTiltDeg(true)).toBe(-45);
  });

  it("封面常态不转、展开转 6°", () => {
    expect(folderCardSheetTiltDeg(false)).toBe(0);
    expect(folderCardSheetTiltDeg(true)).toBe(6);
  });
});

/**
 * 「露出上 2/3」是拿**凸舌肩线**当遮盖线算的，不是前盖顶边：前盖左侧有凸舌，
 * 顶边比肩线高一大截，按顶边算封面会被藏掉一半。这条是整套几何的核心。
 */
describe("封面露出多少：被凸舌肩线盖住下沿，露出上 2/3", () => {
  const sheetHeight = folderCardSheetHeightPx(210);
  const shoulder = folderFlapShoulderY();
  const sheetTop = folderSheetTopPx();

  it("肩线 = 前盖顶边 + 路径里的 41.5404", () => {
    expect(shoulder).toBeCloseTo(23 + 41.5404, 5);
  });

  it("肩线以下的封面高度 = 封面高度的 1/3", () => {
    expect(shoulder - sheetTop).toBeCloseTo((sheetHeight * 2) / 3, 5);
    expect(sheetHeight - (shoulder - sheetTop)).toBeCloseTo(sheetHeight / 3, 5);
  });

  it("封面顶边略高于舞台（纸从文件夹里探出来），底边深埋在前盖后面", () => {
    expect(sheetTop).toBeLessThan(0);
    expect(sheetTop).toBeGreaterThan(-30);
    expect(sheetTop + sheetHeight).toBeGreaterThan(shoulder + 30);
  });
});

/** `mask-image` 用的是同一份轮廓：data-URI 里的 SVG，任意列宽都铺满元素。 */
describe("前盖遮罩：轮廓的 data-URI", () => {
  const mask = folderFlapMaskImage();

  it("是 CSS url()、带自己的 viewBox 并按元素盒子拉伸", () => {
    expect(mask.startsWith("url('data:image/svg+xml,")).toBe(true);
    expect(decodeURIComponent(mask)).toContain('viewBox="0 0 321 241"');
    expect(decodeURIComponent(mask)).toContain('preserveAspectRatio="none"');
  });

  it("里面就是那条逐字的轮廓（遮罩形状不会和墨线跑偏）", () => {
    expect(decodeURIComponent(mask)).toContain(`d="${FOLDER_FLAP_PATH}"`);
  });

  it("URI 里不含裸括号/引号：这个值要穿过 CSS 自定义属性与 style 属性", () => {
    const uri = mask.slice("url('".length, -"')".length);

    expect(uri).not.toMatch(/[()'"]/u);
    expect(uri.startsWith("data:image/svg+xml,")).toBe(true);
  });
});

/**
 * 样式表消费的那一份：几何全部换算成**相对容器的比例**，列宽变了形状不变。
 * 右侧注释是手算的出处（基宽 321 与基高 270 上的像素值 ÷ 基准）。
 */
describe("样式变量：几何换算成百分比与角度（列宽自适应、不写死像素）", () => {
  const vars = folderCardStyleVars();

  it("舞台保持 321 : 270 的比例", () => {
    expect(vars["--folder-stage-ratio"]).toBe("321 / 270");
  });

  it("背板：顶边 10/270、高 260/270（顶边低于凸舌顶、底部留一截后沿）", () => {
    expect(vars["--folder-back-top"]).toBe("3.7%");
    expect(vars["--folder-back-height"]).toBe("96.3%");
  });

  it("封面：宽 210/321、左侧居中、高 118.125/270、顶边 -14.21/270（探出舞台）", () => {
    expect(vars["--folder-sheet-width"]).toBe("65.42%");
    expect(vars["--folder-sheet-left"]).toBe("17.29%");
    expect(vars["--folder-sheet-height"]).toBe("43.75%");
    expect(vars["--folder-sheet-top"]).toBe("-5.26%");
  });

  it("前盖：顶边 23/270、高 241/270（底边 264，留 6 给背板后沿）", () => {
    expect(vars["--folder-flap-top"]).toBe("8.52%");
    expect(vars["--folder-flap-height"]).toBe("89.26%");
  });

  it("抽卡距离按封面高度的比例给（基宽下 30 / 118.125）", () => {
    expect(vars["--folder-sheet-lift"]).toBe("-25.4%");
  });

  it("两个倾角与封面旋转也来自同一份几何（样式表里不写角度字面量）", () => {
    expect(vars["--folder-flap-tilt"]).toBe("-15deg");
    expect(vars["--folder-flap-tilt-open"]).toBe("-45deg");
    expect(vars["--folder-sheet-tilt"]).toBe("6deg");
  });

  it("遮罩以自定义属性下发（样式表只写 mask-image: var(...)）", () => {
    expect(vars["--folder-flap-mask"]).toBe(folderFlapMaskImage());
  });

  it("每次调用返回同一份几何（纯函数、无隐藏状态）", () => {
    expect(folderCardStyleVars()).toEqual(vars);
  });
});
