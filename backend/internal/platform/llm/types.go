package llm

// Message 是一条对话消息。
//
// 不直接暴露 eino 的 schema.Message：业务层只关心角色与文本，
// 不需要为了换框架去改自己的类型。
type Message struct {
	Role    string `json:"role"`    // system / user / assistant
	Content string `json:"content"` // 消息内容
}

// Request 是"发给模型的一次请求"的描述。
//
// 真正拼报文的是 eino 组件，这个类型现在只有两个用途：测试里解析请求体、
// 日志里说明发了什么。保留它是为了让 internal/ai 的对外类型不必跟着框架变。
type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}
