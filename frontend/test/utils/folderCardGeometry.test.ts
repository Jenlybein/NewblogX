import { describe, expect, it } from "vitest";

import {
  FOLDER_CARD_BASE_HEIGHT,
  FOLDER_CARD_BASE_WIDTH,
  folderCardFlapTiltDeg,
  folderCardLiftPx,
  folderCardScale,
  folderCardSheetHeightPx,
  folderCardSheetTiltDeg,
  folderCardStyleVars,
} from "~/utils/folderCardGeometry";

/**
 * 文件夹卡的纯几何（grill 记录 §三 3.6）。
 *
 * 为什么这些数字要抽成纯函数：卡片的几何是**比例**（缩放 = 列宽 / 321），
 * 靠肉眼看样式表只能验"某一种列宽下没坏"。把基宽、抽卡距离、倾角与百分比
 * 换算收成纯函数后，底座上的每一处比例都能用算式钉住，样式表只消费结果。
 *
 * 基宽取参考形态的 321×270：卡里只有**一张** 16:9 封面（不是三张扇形），
 * 封面常态露出上 2/3，前盖从下盖住下沿。
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

describe("封面的 16:9：高度由宽度决定", () => {
  it("236 宽的封面高 132.75（16:9）", () => {
    expect(folderCardSheetHeightPx(236)).toBeCloseTo(132.75, 5);
    expect(folderCardSheetHeightPx(472)).toBeCloseTo(265.5, 5);
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

  it("未展开（无 hover / 无聚焦）时不位移", () => {
    expect(folderCardLiftPx(321, { revealed: false })).toBe(0);
    expect(folderCardLiftPx(642, { revealed: false })).toBe(0);
  });

  it("减少动态下无位移（归零，不是放慢）", () => {
    expect(folderCardLiftPx(321, { reducedMotion: true })).toBe(0);
    expect(folderCardLiftPx(642, { revealed: true, reducedMotion: true })).toBe(0);
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
 * 样式表消费的那一份：几何全部换算成**相对容器的比例**，列宽变了形状不变。
 * 右侧注释是手算的出处（基宽 321 与基高 270 上的像素值 ÷ 基准）。
 */
describe("样式变量：几何换算成百分比与角度（列宽自适应、不写死像素）", () => {
  const vars = folderCardStyleVars();

  it("舞台保持 321 : 270 的比例", () => {
    expect(vars["--folder-stage-ratio"]).toBe("321 / 270");
  });

  it("封面：宽度 236/321、左侧居中、顶部 46/270、高度 132.75/270", () => {
    expect(vars["--folder-sheet-width"]).toBe("73.52%");
    expect(vars["--folder-sheet-left"]).toBe("13.24%");
    expect(vars["--folder-sheet-top"]).toBe("17.04%");
    expect(vars["--folder-sheet-height"]).toBe("49.17%");
  });

  it("背板：8..313 宽、12..264 高（封面完全落在背板底色上）", () => {
    expect(vars["--folder-back-left"]).toBe("2.49%");
    expect(vars["--folder-back-top"]).toBe("4.44%");
    expect(vars["--folder-back-width"]).toBe("95.02%");
    expect(vars["--folder-back-height"]).toBe("93.33%");
  });

  it("前盖：顶边 = 封面顶 + 2/3 封面高（盖住下沿、露出上 2/3），底边与背板齐平", () => {
    expect(vars["--folder-flap-left"]).toBe("3.12%");
    expect(vars["--folder-flap-top"]).toBe("49.81%");
    expect(vars["--folder-flap-width"]).toBe("93.77%");
    expect(vars["--folder-flap-height"]).toBe("47.96%");
  });

  it("抽卡距离按封面高度的比例给（基宽下 30 / 132.75 = 22.6%）", () => {
    expect(vars["--folder-sheet-lift"]).toBe("-22.6%");
  });

  it("两个倾角与封面旋转也来自同一份几何（样式表里不写角度字面量）", () => {
    expect(vars["--folder-flap-tilt"]).toBe("-15deg");
    expect(vars["--folder-flap-tilt-open"]).toBe("-45deg");
    expect(vars["--folder-sheet-tilt"]).toBe("6deg");
  });

  it("每次调用返回同一份几何（纯函数、无隐藏状态）", () => {
    expect(folderCardStyleVars()).toEqual(vars);
  });
});
