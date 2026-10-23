import { describe, expect, it } from "vitest";
import { buildAiConversationPrompt, parseAiSseDataBlock, resolveAiRequestUrl } from "~/services/ai";

describe("ai service helpers", () => {
  it("returns the latest input directly when there is no history", () => {
    expect(buildAiConversationPrompt([], "  帮我找 Nuxt 文章  ")).toBe("帮我找 Nuxt 文章");
  });

  it("builds a compact transcript prompt when recent history exists", () => {
    const prompt = buildAiConversationPrompt(
      [
        { role: "user", content: "第一问" },
        { role: "assistant", content: "第一答" },
        { role: "user", content: "第二问" },
      ],
      "继续展开说说",
    );

    expect(prompt).toContain("用户：第一问");
    expect(prompt).toContain("助手：第一答");
    expect(prompt).toContain("用户：第二问");
    expect(prompt).toContain("用户：继续展开说说");
  });

  it("parses ai sse data blocks into API envelopes", () => {
    const payload = parseAiSseDataBlock('event: message\ndata: {"code":0,"data":{"content":"你好"},"msg":"成功"}');

    expect(payload).toEqual({
      code: 0,
      data: {
        content: "你好",
      },
      msg: "成功",
    });
  });

  it("returns null for empty sse blocks", () => {
    expect(parseAiSseDataBlock("event: message")).toBeNull();
  });

  it("deduplicates api prefixes when resolving ai request urls", () => {
    expect(resolveAiRequestUrl("/api", "/api/ai/search/llm")).toBe("/api/ai/search/llm");
    expect(resolveAiRequestUrl("/_backend", "/api/ai/search/llm")).toBe("/_backend/api/ai/search/llm");
    expect(resolveAiRequestUrl("/_backend/api", "/api/ai/search/llm")).toBe("/_backend/api/ai/search/llm");
  });
});
