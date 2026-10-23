declare module "~/services/mermaid-runtime.mjs" {
  export const SUPPORTED_MERMAID_DIAGRAMS: readonly string[];
  export function detectSupportedMermaidDiagram(
    text: string,
    config?: Record<string, unknown>,
  ): string | null;

  export interface MermaidRuntime {
    initialize: (config: { startOnLoad: boolean; securityLevel: string }) => void;
    render: (
      id: string,
      text: string,
      container?: Element | null,
    ) => Promise<{
      diagramType: string;
      svg: string;
      bindFunctions?: (element: Element) => void;
    }>;
  }

  const runtime: MermaidRuntime;
  export default runtime;
}
