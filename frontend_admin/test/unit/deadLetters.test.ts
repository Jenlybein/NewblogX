import { describe, expect, it } from "vitest";
import {
  canReplay,
  deadLetterStatusLabel,
  replayResultLabel,
  replayResultTagType,
  streamLabel,
} from "~/utils/deadLetters";

describe("streamLabel", () => {
  it("已知的流有中文名", () => {
    expect(streamLabel("es_river")).toBe("搜索同步（ES 直投）");
    expect(streamLabel("outbox_relay")).toBe("领域事件（发件箱）");
  });

  it("未知的流原样返回，不吞掉信息", () => {
    expect(streamLabel("some_new_stream")).toBe("some_new_stream");
  });

  it("空值显示为占位符", () => {
    expect(streamLabel("")).toBe("-");
    expect(streamLabel("   ")).toBe("-");
  });
});

describe("deadLetterStatusLabel", () => {
  it("两个状态都有中文名", () => {
    expect(deadLetterStatusLabel("pending")).toBe("待处理");
    expect(deadLetterStatusLabel("resolved")).toBe("已解决");
  });

  it("未知状态原样返回", () => {
    expect(deadLetterStatusLabel("weird")).toBe("weird");
  });
});

describe("replayResultLabel / replayResultTagType", () => {
  it("四种结果都有中文名", () => {
    expect(replayResultLabel("success")).toBe("成功");
    expect(replayResultLabel("failed")).toBe("失败");
    expect(replayResultLabel("skipped")).toBe("跳过");
    expect(replayResultLabel("unsupported")).toBe("不支持重投");
  });

  it("颜色映射：失败是错的、跳过是提醒", () => {
    expect(replayResultTagType("success")).toBe("success");
    expect(replayResultTagType("failed")).toBe("error");
    expect(replayResultTagType("skipped")).toBe("warning");
    expect(replayResultTagType("unsupported")).toBe("warning");
    expect(replayResultTagType("whatever")).toBe("default");
  });
});

describe("canReplay", () => {
  it("只有已解决的不该再点", () => {
    expect(canReplay("pending")).toBe(true);
    expect(canReplay("resolved")).toBe(false);
    expect(canReplay("")).toBe(true);
  });
});
