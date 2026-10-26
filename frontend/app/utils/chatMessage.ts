// 私信消息的"内容是什么"——把后端存的 content 翻成前端能直接渲染的东西。
//
// 为什么需要这一层：后端对不同类型的消息，content 里装的东西不一样。
// 尤其是图片消息，后端会把传入的图片地址重新包成一段 JSON 再落库：
//
//   发送时 content = "https://cdn/a.png"
//   库里 content   = {"kind":"image","image_url":"https://cdn/a.png"}
//
// 如果前端直接把 content 当文本渲染，用户看到的就是一坨 JSON。
// 这类"传输格式"与"展示内容"的差异常常被散落在组件里用 if 处理，
// 结果每加一种消息类型就要改所有渲染点。收成一个纯函数，改一处即可。

/** 可发送的消息类型（后端 binding: oneof=1 2 7）。 */
export type ChatSendType = 1 | 2 | 7;

/** 消息内容解析后的形态。 */
export type ChatMessageKind = "text" | "image" | "markdown";

export type ResolvedChatMessage = {
  kind: ChatMessageKind;
  /** 文本/Markdown 消息的可渲染原文；图片消息这里是空串。 */
  text: string;
  /** 图片消息的地址；非图片消息这里是空串。 */
  imageUrl: string;
};

/** 后端在图片消息 content 里写的 JSON 结构（只取我们关心的字段）。 */
type ImagePayload = {
  kind?: string;
  image_url?: string;
};

/**
 * 解析一条消息的内容。
 *
 * 容错策略是"尽量给出能显示的东西"：
 *   - 图片消息若不是合法 JSON（老数据、别处写入的），把整个 content 当地址用；
 *   - 拿不到地址时退化成文本，绝不让气泡空着——空气泡比显示一串 JSON 更难排查。
 */
export function resolveChatMessageContent(item: { msg_type?: number; content?: string }): ResolvedChatMessage {
  const content = typeof item.content === "string" ? item.content : "";

  if (item.msg_type === 2) {
    const imageUrl = extractImageUrl(content);
    if (imageUrl) {
      return { kind: "image", text: "", imageUrl };
    }
    return { kind: "text", text: content, imageUrl: "" };
  }

  if (item.msg_type === 7) {
    return { kind: "markdown", text: content, imageUrl: "" };
  }

  return { kind: "text", text: content, imageUrl: "" };
}

/** 从图片消息的 content 里取出地址；既接受 JSON，也接受裸地址。 */
function extractImageUrl(content: string): string {
  const trimmed = content.trim();
  if (!trimmed) {
    return "";
  }

  if (trimmed.startsWith("{")) {
    try {
      const parsed = JSON.parse(trimmed) as ImagePayload;
      const url = typeof parsed.image_url === "string" ? parsed.image_url.trim() : "";
      return url;
    } catch {
      return "";
    }
  }

  return trimmed;
}

/** 会话列表里"最后一条消息"的展示文本（后端已给摘要，这里只兜底）。 */
export function describeChatMessage(item: { msg_type?: number; content?: string }): string {
  const resolved = resolveChatMessageContent(item);
  if (resolved.kind === "image") {
    return "[图片]";
  }
  return resolved.text;
}
