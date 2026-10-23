const frontMatterRegex = /^-{3}\s*[\n\r](.*?)[\n\r]-{3}\s*[\n\r]+/su;
const directiveRegex =
  /%{2}\{\s*(?:(\w+)\s*:|(\w+))\s*(?:(\w+)|((?:(?!\}%{2}).|\r?\n)*))?\s*(?:\}%{2})?/giu;
const commentRegex = /^\s*%%(?!\{)[^\n]+\n?/gmu;

export type MermaidRuntimeConfig = {
  flowchart?: {
    defaultRenderer?: string;
  };
  class?: {
    defaultRenderer?: string;
  };
  state?: {
    defaultRenderer?: string;
  };
  layout?: string;
};

type MermaidDiagramDefinition = {
  id: string;
  detector: (text: string, config?: MermaidRuntimeConfig) => boolean;
};

export const SUPPORTED_MERMAID_DIAGRAMS = Object.freeze([
  "flowchart",
  "flowchart-v2",
  "sequence",
  "classDiagram",
  "classDiagram-v2",
  "stateDiagram",
  "stateDiagram-v2",
  "er",
  "journey",
  "gantt",
  "pie",
]);

export const supportedDiagramDetectors: readonly MermaidDiagramDefinition[] = Object.freeze([
  {
    id: "classDiagram-v2",
    detector: (text, config) => {
      if (/^\s*classDiagram/.test(text) && config?.class?.defaultRenderer === "dagre-wrapper") {
        return true;
      }
      return /^\s*classDiagram-v2/.test(text);
    },
  },
  {
    id: "classDiagram",
    detector: (text, config) => {
      if (/^\s*classDiagram/.test(text) && config?.class?.defaultRenderer === "dagre-wrapper") {
        return false;
      }
      return /^\s*classDiagram/.test(text);
    },
  },
  {
    id: "er",
    detector: (text) => /^\s*erDiagram/.test(text),
  },
  {
    id: "gantt",
    detector: (text) => /^\s*gantt/.test(text),
  },
  {
    id: "pie",
    detector: (text) => /^\s*pie/.test(text),
  },
  {
    id: "sequence",
    detector: (text) => /^\s*sequenceDiagram/.test(text),
  },
  {
    id: "flowchart-v2",
    detector: (text, config) => {
      if (config?.flowchart?.defaultRenderer === "dagre-d3") {
        return false;
      }
      if (/^\s*graph/.test(text) && config?.flowchart?.defaultRenderer === "dagre-wrapper") {
        return true;
      }
      return /^\s*flowchart/.test(text);
    },
  },
  {
    id: "flowchart",
    detector: (text, config) => {
      if (config?.flowchart?.defaultRenderer === "dagre-wrapper" || config?.flowchart?.defaultRenderer === "elk") {
        return false;
      }
      return /^\s*graph/.test(text);
    },
  },
  {
    id: "stateDiagram-v2",
    detector: (text, config) => {
      if (/^\s*stateDiagram-v2/.test(text)) {
        return true;
      }
      if (/^\s*stateDiagram/.test(text) && config?.state?.defaultRenderer === "dagre-wrapper") {
        return true;
      }
      return false;
    },
  },
  {
    id: "stateDiagram",
    detector: (text, config) => {
      if (/^\s*stateDiagram-v2/.test(text)) {
        return false;
      }
      if (/^\s*stateDiagram/.test(text) && config?.state?.defaultRenderer === "dagre-wrapper") {
        return false;
      }
      return /^\s*stateDiagram/.test(text);
    },
  },
  {
    id: "journey",
    detector: (text) => /^\s*journey/.test(text),
  },
]);

function normalizeDiagramSource(text: string) {
  return text.replace(frontMatterRegex, "").replace(directiveRegex, "").replace(commentRegex, "").trimStart();
}

export function detectSupportedMermaidDiagram(text: string, config: MermaidRuntimeConfig = {}) {
  const normalizedText = normalizeDiagramSource(text);
  for (const { id, detector } of supportedDiagramDetectors) {
    if (detector(normalizedText, config)) {
      return id;
    }
  }
  return null;
}
