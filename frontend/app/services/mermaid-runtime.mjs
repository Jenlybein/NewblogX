/**
 * Mermaid 运行时封装。
 *
 * 只依赖 mermaid 的公开 API（mermaid.initialize / mermaid.render），不再 import
 * dist/chunks/ 下带内容哈希的内部实现文件。
 *
 * 旧做法为了“只注册编辑器支持的少数图表”而深挖内部 API，代价是构建被钉死在某个
 * 精确版本：mermaid 一升级、内部 chunk 哈希变化，构建就会失败，而 lint / 类型检查
 * 都发现不了。现在改用公开 API，mermaid 会注册全部图表类型（bundle 略大），换来
 * 版本稳定、可以正常升级。编辑器侧的图表白名单仍保留在 mermaid-diagram-support.ts。
 */
import DOMPurify from "dompurify";
import mermaid from "mermaid";
import {
  SUPPORTED_MERMAID_DIAGRAMS,
  detectSupportedMermaidDiagram,
} from "./mermaid-diagram-support.ts";

// mermaid 自身按 securityLevel 做了清洗，这里再兜一层；放行项沿用旧实现，
// 避免 DOMPurify 把 foreignObject 这类合法节点裁掉。
const DOMPURIFY_TAGS = ["foreignobject"];
const DOMPURIFY_ATTR = ["dominant-baseline"];

export { SUPPORTED_MERMAID_DIAGRAMS, detectSupportedMermaidDiagram };

function sanitizeSvg(svg) {
  if (!svg || typeof window === "undefined") {
    return svg;
  }
  return DOMPurify.sanitize(svg, {
    ADD_TAGS: DOMPURIFY_TAGS,
    ADD_ATTR: DOMPURIFY_ATTR,
    HTML_INTEGRATION_POINTS: { foreignobject: true },
  });
}

// 与旧接口保持一致：config 里的 startOnLoad / securityLevel 等原样透传给 mermaid。
function initialize(userOptions = {}) {
  mermaid.initialize({
    startOnLoad: false,
    ...userOptions,
  });
}

// 与旧接口保持一致：返回 { diagramType, svg, bindFunctions }。
async function render(id, text, svgContainingElement) {
  const result = svgContainingElement
    ? await mermaid.render(id, text, svgContainingElement)
    : await mermaid.render(id, text);

  return {
    diagramType: result.diagramType ?? "unknown",
    svg: sanitizeSvg(result.svg),
    bindFunctions: result.bindFunctions,
  };
}

export default {
  initialize,
  render,
};
