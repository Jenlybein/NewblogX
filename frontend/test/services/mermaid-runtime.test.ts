import { describe, expect, it } from "vitest";
import {
  SUPPORTED_MERMAID_DIAGRAMS,
  detectSupportedMermaidDiagram,
} from "~/services/mermaid-diagram-support";

describe("mermaid runtime whitelist", () => {
  it("locks the supported diagram whitelist to the editor-facing set", () => {
    expect(SUPPORTED_MERMAID_DIAGRAMS).toEqual([
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
  });

  it("detects each supported diagram type", () => {
    expect(detectSupportedMermaidDiagram("graph TD\nA --> B")).toBe("flowchart");
    expect(detectSupportedMermaidDiagram("flowchart LR\nA --> B")).toBe("flowchart-v2");
    expect(detectSupportedMermaidDiagram("sequenceDiagram\nA->>B: hi")).toBe("sequence");
    expect(detectSupportedMermaidDiagram("classDiagram\nclass Article")).toBe("classDiagram");
    expect(detectSupportedMermaidDiagram("classDiagram-v2\nclass Article")).toBe("classDiagram-v2");
    expect(detectSupportedMermaidDiagram("stateDiagram\n[*] --> Draft")).toBe("stateDiagram");
    expect(detectSupportedMermaidDiagram("stateDiagram-v2\n[*] --> Draft")).toBe("stateDiagram-v2");
    expect(detectSupportedMermaidDiagram("erDiagram\nUSER ||--o{ POST : writes")).toBe("er");
    expect(detectSupportedMermaidDiagram("journey\ntitle Publish")).toBe("journey");
    expect(detectSupportedMermaidDiagram("gantt\ntitle Launch")).toBe("gantt");
    expect(detectSupportedMermaidDiagram("pie\ntitle Share")).toBe("pie");
  });

  it("keeps renderer-specific v2 detection behavior explicit", () => {
    expect(
      detectSupportedMermaidDiagram("classDiagram\nclass Article", {
        class: { defaultRenderer: "dagre-wrapper" },
      }),
    ).toBe("classDiagram-v2");

    expect(
      detectSupportedMermaidDiagram("stateDiagram\n[*] --> Draft", {
        state: { defaultRenderer: "dagre-wrapper" },
      }),
    ).toBe("stateDiagram-v2");

    expect(
      detectSupportedMermaidDiagram("graph TD\nA --> B", {
        flowchart: { defaultRenderer: "dagre-d3" },
      }),
    ).toBe("flowchart");
  });

  it("rejects unsupported mermaid diagram families", () => {
    expect(detectSupportedMermaidDiagram("mindmap\n  root((BlogX))")).toBeNull();
    expect(detectSupportedMermaidDiagram("architecture-beta\nservice api(cloud)[API]")).toBeNull();
    expect(detectSupportedMermaidDiagram("requirementDiagram\nrequirement demo")).toBeNull();
    expect(detectSupportedMermaidDiagram("sankey-beta\nA,B,1")).toBeNull();
  });

  it("ignores mermaid directives and comments before detection", () => {
    const source = `%%{init: { "theme": "default" }}%%
%% comment
flowchart TD
  A --> B`;

    expect(detectSupportedMermaidDiagram(source)).toBe("flowchart-v2");
  });
});
