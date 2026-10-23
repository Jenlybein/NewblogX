declare module "markdown-it" {
  type MarkdownPlugin = (md: MarkdownIt, ...params: unknown[]) => void;

  interface MarkdownToken {
    type: string;
    tag: string;
    info?: string;
    content: string;
    attrSet(name: string, value: string): void;
  }

  interface MarkdownItOptions {
    html?: boolean;
    xhtmlOut?: boolean;
    breaks?: boolean;
    linkify?: boolean;
    typographer?: boolean;
    langPrefix?: string;
    quotes?: string;
    highlight?: (code: string, language: string) => string;
  }

  export default class MarkdownIt {
    renderer: {
      render(tokens: unknown[], options: unknown, env: Record<string, unknown>): string;
      rules: {
        fence?: (
          tokens: Array<{ info?: string; content: string }>,
          idx: number,
          options: unknown,
          env: unknown,
          self: { renderToken: (tokens: unknown[], idx: number, options: unknown) => string },
        ) => string;
      };
    };
    options: unknown;

    constructor(options?: MarkdownItOptions);
    use(plugin: MarkdownPlugin, ...params: unknown[]): this;
    parse(source: string, env: Record<string, unknown>): MarkdownToken[];
    render(source: string): string;
  }
}

declare module "#markdown-it" {
  export { default } from "markdown-it";
}

declare module "markdown-it-katex" {
  import type MarkdownIt from "markdown-it";

  export default function markdownItKatex(md: MarkdownIt, ...params: unknown[]): void;
}

declare module "markdown-it-ins" {
  import type MarkdownIt from "markdown-it";

  export default function markdownItIns(md: MarkdownIt, ...params: unknown[]): void;
}

declare module "#markdown-it-ins" {
  export { default } from "markdown-it-ins";
}
