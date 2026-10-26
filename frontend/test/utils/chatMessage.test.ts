import { describe, expect, it } from "vitest";

import { describeChatMessage, resolveChatMessageContent } from "~/utils/chatMessage";

// 消息内容解析的用例。
//
// 这里钉的是"后端存的 content"到"前端能显示的东西"这一段映射。
// 之所以值得单独测：图片消息的 content 是**后端二次包装过的 JSON**，
// 这个约定不在前端代码里、也不在类型里，一旦后端改包装格式
// （比如把 image_url 改名），前端只会表现为"图片变成一坨 JSON 文本"，
// 不会报错、不会有类型错误。只有这种逐条断言能拦住。

describe("resolveChatMessageContent · 文本", () => {
  it("msg_type=1 直接当纯文本", () => {
    expect(resolveChatMessageContent({ msg_type: 1, content: "你好" })).toEqual({
      kind: "text",
      text: "你好",
      imageUrl: "",
    });
  });

  it("msg_type 缺失或没见过的类型按文本处理", () => {
    expect(resolveChatMessageContent({ content: "abc" }).kind).toBe("text");
    expect(resolveChatMessageContent({ msg_type: 99, content: "abc" }).kind).toBe("text");
  });

  it("content 不是字符串时不炸", () => {
    expect(resolveChatMessageContent({ msg_type: 1 }).text).toBe("");
  });
});

describe("resolveChatMessageContent · 图片", () => {
  it("识别后端包装的 JSON，取出 image_url", () => {
    const result = resolveChatMessageContent({
      msg_type: 2,
      content: '{"kind":"image","image_url":"https://cdn.example.com/a.png"}',
    });

    expect(result).toEqual({
      kind: "image",
      text: "",
      imageUrl: "https://cdn.example.com/a.png",
    });
  });

  it("裸地址（尚未落库的乐观消息）也能显示", () => {
    const result = resolveChatMessageContent({ msg_type: 2, content: "https://cdn.example.com/a.png" });

    expect(result.kind).toBe("image");
    expect(result.imageUrl).toBe("https://cdn.example.com/a.png");
  });

  it("JSON 里没有 image_url 时退化成文本，而不是显示空气泡", () => {
    const result = resolveChatMessageContent({ msg_type: 2, content: '{"kind":"image"}' });

    expect(result.kind).toBe("text");
    expect(result.text).toBe('{"kind":"image"}');
  });

  it("看起来像 JSON 但解析失败时同样退化成文本", () => {
    const result = resolveChatMessageContent({ msg_type: 2, content: "{not-json" });

    expect(result.kind).toBe("text");
    expect(result.text).toBe("{not-json");
  });

  it("空内容的图片消息不会给出空地址", () => {
    expect(resolveChatMessageContent({ msg_type: 2, content: "   " }).imageUrl).toBe("");
  });
});

describe("resolveChatMessageContent · Markdown", () => {
  it("msg_type=7 保留原文交给渲染器", () => {
    const result = resolveChatMessageContent({ msg_type: 7, content: "# 标题\n\n正文" });

    expect(result).toEqual({ kind: "markdown", text: "# 标题\n\n正文", imageUrl: "" });
  });
});

describe("describeChatMessage", () => {
  it("图片消息统一显示「[图片]」（与后端会话摘要一致）", () => {
    expect(describeChatMessage({ msg_type: 2, content: '{"kind":"image","image_url":"x"}' })).toBe("[图片]");
  });

  it("其余类型返回可显示的文本", () => {
    expect(describeChatMessage({ msg_type: 1, content: "晚上好" })).toBe("晚上好");
  });
});
