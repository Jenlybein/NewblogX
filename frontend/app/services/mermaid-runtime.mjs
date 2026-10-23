import { select } from "d3";
import DOMPurify from "dompurify";
import isEmpty from "lodash-es/isEmpty.js";
import { compile, serialize, stringify } from "stylis";
import {
  SUPPORTED_MERMAID_DIAGRAMS,
  detectSupportedMermaidDiagram,
  supportedDiagramDetectors,
} from "./mermaid-diagram-support.ts";
import { log, setLogLevel } from "mermaid/dist/chunks/mermaid.core/chunk-AGHRB4JF.mjs";
import {
  UnknownDiagramError,
  addDirective,
  assignWithDepth_default,
  detectType,
  evaluate,
  frontMatterRegex,
  getConfig,
  getDiagram,
  getDiagramLoader,
  getEffectiveHtmlLabels,
  registerDiagram,
  registerLazyLoadedDiagrams,
  reset,
  saveConfigFromInitialize,
  setSiteConfig,
  styles_default,
  themes_default,
} from "mermaid/dist/chunks/mermaid.core/chunk-ICPOFSXX.mjs";
import {
  cleanAndMerge,
  decodeEntities,
  encodeEntities,
  removeDirectives,
  utils_default,
} from "mermaid/dist/chunks/mermaid.core/chunk-5PVQY5BW.mjs";
import { JSON_SCHEMA, load } from "mermaid/dist/chunks/mermaid.core/chunk-XPW4576I.mjs";

// This runtime is intentionally pinned to Mermaid 11.14.0 internals so we can
// keep only the diagram loaders that the editor actually exposes.
const MAX_TEXTLENGTH = 5e4;
const MAX_TEXTLENGTH_EXCEEDED_MSG = "graph TB;a[Maximum text size in diagram exceeded];style a fill:#faa";
const XMLNS_SVG_STD = "http://www.w3.org/2000/svg";
const XMLNS_XLINK_STD = "http://www.w3.org/1999/xlink";
const XMLNS_XHTML_STD = "http://www.w3.org/1999/xhtml";
const DOMPURIFY_TAGS = ["foreignobject"];
const DOMPURIFY_ATTR = ["dominant-baseline"];
export { SUPPORTED_MERMAID_DIAGRAMS, detectSupportedMermaidDiagram };

let diagramsRegistered = false;

function getSupportedDetector(id) {
  const definition = supportedDiagramDetectors.find((item) => item.id === id);
  if (!definition) {
    throw new UnknownDiagramError(`Diagram ${id} is not configured in the local whitelist.`);
  }
  return definition.detector;
}

const flowchartDetector = {
  id: "flowchart",
  detector: getSupportedDetector("flowchart"),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/flowDiagram-DWJPFMVM.mjs");
    return { id: "flowchart", diagram };
  },
};

const flowchartV2Detector = {
  id: "flowchart-v2",
  detector: (text, config) => {
    if (config?.flowchart?.defaultRenderer === "elk") {
      config.layout = "elk";
    }
    return getSupportedDetector("flowchart-v2")(text, config);
  },
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/flowDiagram-DWJPFMVM.mjs");
    return { id: "flowchart-v2", diagram };
  },
};

const erDetector = {
  id: "er",
  detector: (text) => /^\s*erDiagram/.test(text),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/erDiagram-SMLLAGMA.mjs");
    return { id: "er", diagram };
  },
};

const ganttDetector = {
  id: "gantt",
  detector: (text) => /^\s*gantt/.test(text),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/ganttDiagram-T4ZO3ILL.mjs");
    return { id: "gantt", diagram };
  },
};

const pieDetector = {
  id: "pie",
  detector: (text) => /^\s*pie/.test(text),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/pieDiagram-DEJITSTG.mjs");
    return { id: "pie", diagram };
  },
};

const sequenceDetector = {
  id: "sequence",
  detector: (text) => /^\s*sequenceDiagram/.test(text),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/sequenceDiagram-FGHM5R23.mjs");
    return { id: "sequence", diagram };
  },
};

const classDiagramDetector = {
  id: "classDiagram",
  detector: getSupportedDetector("classDiagram"),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/classDiagram-6PBFFD2Q.mjs");
    return { id: "classDiagram", diagram };
  },
};

const classDiagramV2Detector = {
  id: "classDiagram-v2",
  detector: getSupportedDetector("classDiagram-v2"),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/classDiagram-v2-HSJHXN6E.mjs");
    return { id: "classDiagram-v2", diagram };
  },
};

const stateDiagramDetector = {
  id: "stateDiagram",
  detector: getSupportedDetector("stateDiagram"),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/stateDiagram-FHFEXIEX.mjs");
    return { id: "stateDiagram", diagram };
  },
};

const stateDiagramV2Detector = {
  id: "stateDiagram-v2",
  detector: getSupportedDetector("stateDiagram-v2"),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/stateDiagram-v2-QKLJ7IA2.mjs");
    return { id: "stateDiagram-v2", diagram };
  },
};

const journeyDetector = {
  id: "journey",
  detector: (text) => /^\s*journey/.test(text),
  loader: async () => {
    const { diagram } = await import("mermaid/dist/chunks/mermaid.core/journeyDiagram-VCZTEJTY.mjs");
    return { id: "journey", diagram };
  },
};

const supportedDiagramDefinitions = Object.freeze([
  classDiagramV2Detector,
  classDiagramDetector,
  erDetector,
  ganttDetector,
  pieDetector,
  sequenceDetector,
  flowchartV2Detector,
  flowchartDetector,
  stateDiagramV2Detector,
  stateDiagramDetector,
  journeyDetector,
]);

function addSupportedDiagrams() {
  if (diagramsRegistered) {
    return;
  }

  diagramsRegistered = true;
  registerLazyLoadedDiagrams(...supportedDiagramDefinitions);
}

function cleanupComments(text) {
  return text.replace(/^\s*%%(?!{)[^\n]+\n?/gm, "").trimStart();
}

function extractFrontMatter(text) {
  const matches = text.match(frontMatterRegex);
  if (!matches) {
    return {
      text,
      metadata: {},
    };
  }

  let parsed = load(matches[1], { schema: JSON_SCHEMA }) ?? {};
  parsed = typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};

  const metadata = {};
  if (parsed.displayMode) {
    metadata.displayMode = parsed.displayMode.toString();
  }
  if (parsed.title) {
    metadata.title = parsed.title.toString();
  }
  if (parsed.config) {
    metadata.config = parsed.config;
  }

  return {
    text: text.slice(matches[0].length),
    metadata,
  };
}

function cleanupText(code) {
  return code.replace(/\r\n?/g, "\n").replace(/<(\w+)([^>]*)>/g, (_match, tag, attributes) => {
    return "<" + tag + attributes.replace(/=\"([^\"]*)\"/g, "='$1'") + ">";
  });
}

function processFrontmatter(code) {
  const { text, metadata } = extractFrontMatter(code);
  const { displayMode, title, config = {} } = metadata;

  if (displayMode) {
    if (!config.gantt) {
      config.gantt = {};
    }
    config.gantt.displayMode = displayMode;
  }

  return { title, config, text };
}

function processDirectives(code) {
  const initDirective = utils_default.detectInit(code) ?? {};
  const wrapDirectives = utils_default.detectDirective(code, "wrap");

  if (Array.isArray(wrapDirectives)) {
    initDirective.wrap = wrapDirectives.some(({ type }) => type === "wrap");
  } else if (wrapDirectives?.type === "wrap") {
    initDirective.wrap = true;
  }

  return {
    text: removeDirectives(code),
    directive: initDirective,
  };
}

function preprocessDiagram(code) {
  const cleanedCode = cleanupText(code);
  const frontMatterResult = processFrontmatter(cleanedCode);
  const directiveResult = processDirectives(frontMatterResult.text);
  const config = cleanAndMerge(frontMatterResult.config, directiveResult.directive);

  return {
    code: cleanupComments(directiveResult.text),
    title: frontMatterResult.title,
    config,
  };
}

function processAndSetConfigs(text) {
  const processed = preprocessDiagram(text);
  reset();
  addDirective(processed.config ?? {});
  return processed;
}

function createCssStyles(config, classDefs = new Map()) {
  let cssStyles = "";

  if (config.themeCSS !== undefined) {
    cssStyles += `\n${config.themeCSS}`;
  }
  if (config.fontFamily !== undefined) {
    cssStyles += `\n:root { --mermaid-font-family: ${config.fontFamily}}`;
  }
  if (config.altFontFamily !== undefined) {
    cssStyles += `\n:root { --mermaid-alt-font-family: ${config.altFontFamily}}`;
  }
  if (!(classDefs instanceof Map)) {
    return cssStyles;
  }

  const htmlLabels = getEffectiveHtmlLabels(config);
  const cssElements = htmlLabels ? ["> *", "span"] : ["rect", "polygon", "ellipse", "circle", "path"];

  classDefs.forEach((styleClassDef) => {
    if (!isEmpty(styleClassDef.styles)) {
      cssElements.forEach((cssElement) => {
        cssStyles += `\n.${styleClassDef.id} ${cssElement} { ${(styleClassDef.styles || []).join(" !important; ")} !important; }`;
      });
    }
    if (!isEmpty(styleClassDef.textStyles)) {
      cssStyles += `\n.${styleClassDef.id} tspan { ${(
        styleClassDef.textStyles || []
      ).map((style) => style.replace("color", "fill")).join(" !important; ")} !important; }`;
    }
  });

  return cssStyles;
}

function createUserStyles(config, graphType, classDefs, svgId) {
  const userCSSstyles = createCssStyles(config, classDefs);
  const allStyles = styles_default(
    graphType,
    userCSSstyles,
    { ...config.themeVariables, theme: config.theme, look: config.look },
    svgId,
  );
  return serialize(compile(`${svgId}{${allStyles}}`), stringify);
}

function cleanUpSvgCode(svgCode = "", useArrowMarkerUrls) {
  let cleanedSvg = svgCode;
  if (!useArrowMarkerUrls) {
    cleanedSvg = cleanedSvg.replace(/marker-end="url\([\d+./:=?A-Za-z-]*?#/g, 'marker-end="url(#');
  }
  cleanedSvg = decodeEntities(cleanedSvg);
  return cleanedSvg.replace(/<br>/g, "<br/>");
}

function appendDivSvgG(parentRoot, id, enclosingDivId, divStyle, svgXlink) {
  const enclosingDiv = parentRoot.append("div");
  enclosingDiv.attr("id", enclosingDivId);
  if (divStyle) {
    enclosingDiv.attr("style", divStyle);
  }

  const svgNode = enclosingDiv.append("svg").attr("id", id).attr("width", "100%").attr("xmlns", XMLNS_SVG_STD);
  if (svgXlink) {
    svgNode.attr("xmlns:xlink", svgXlink);
  }
  svgNode.append("g");
}

function removeExistingElements(doc, id, divId) {
  doc.getElementById(id)?.remove();
  doc.getElementById(divId)?.remove();
}

class Diagram {
  constructor(type, text, db, parser, renderer) {
    this.type = type;
    this.text = text;
    this.db = db;
    this.parser = parser;
    this.renderer = renderer;
  }

  static async fromText(text, metadata = {}) {
    const config = getConfig();
    const type = detectType(text, config);
    const encodedText = `${encodeEntities(text)}\n`;

    try {
      getDiagram(type);
    } catch {
      const loader = getDiagramLoader(type);
      if (!loader) {
        throw new UnknownDiagramError(`Diagram ${type} not found.`);
      }
      const { id, diagram } = await loader();
      registerDiagram(id, diagram);
    }

    const { db, parser, renderer, init } = getDiagram(type);
    if (parser.parser) {
      parser.parser.yy = db;
    }

    db.clear?.();
    init?.(config);
    if (metadata.title) {
      db.setDiagramTitle?.(metadata.title);
    }

    await parser.parse(encodedText);
    return new Diagram(type, encodedText, db, parser, renderer);
  }
}

async function render(id, text, svgContainingElement) {
  addSupportedDiagrams();

  const processed = processAndSetConfigs(text);
  const source = processed.code.length > (getConfig()?.maxTextSize ?? MAX_TEXTLENGTH) ? MAX_TEXTLENGTH_EXCEEDED_MSG : processed.code;
  const config = getConfig();
  const idSelector = `#${id}`;
  const enclosingDivID = `d${id}`;
  const enclosingDivIDSelector = `#${enclosingDivID}`;
  const removeTempElements = () => {
    const node = select(enclosingDivIDSelector).node();
    if (node && "remove" in node) {
      node.remove();
    }
  };

  let root = select("body");
  const fontFamily = config.fontFamily;

  if (svgContainingElement !== undefined) {
    if (svgContainingElement) {
      svgContainingElement.innerHTML = "";
      root = select(svgContainingElement);
      appendDivSvgG(root, id, enclosingDivID, `font-family: ${fontFamily}`, XMLNS_XLINK_STD);
    } else {
      removeExistingElements(document, id, enclosingDivID);
      appendDivSvgG(root, id, enclosingDivID);
    }
  } else {
    removeExistingElements(document, id, enclosingDivID);
    appendDivSvgG(root, id, enclosingDivID);
  }

  log.debug(config);
  const diagram = await Diagram.fromText(source, { title: processed.title });
  const element = root.select(enclosingDivIDSelector).node();
  const svg = element?.firstChild;
  const firstChild = svg?.firstChild;
  const diagramClassDefs = diagram.renderer.getClasses?.(source, diagram);
  const rules = createUserStyles(config, diagram.type, diagramClassDefs, idSelector);
  const styleNode = document.createElement("style");
  styleNode.innerHTML = rules;
  svg?.insertBefore(styleNode, firstChild || null);

  await diagram.renderer.draw(source, id, "11.14.0", diagram);

  root.select(`[id="${id}"]`).selectAll("foreignobject > *").attr("xmlns", XMLNS_XHTML_STD);

  let svgCode = root.select(enclosingDivIDSelector).node()?.innerHTML || "";
  svgCode = cleanUpSvgCode(svgCode, evaluate(config.arrowMarkerAbsolute));
  svgCode = DOMPurify.sanitize(svgCode, {
    ADD_TAGS: DOMPURIFY_TAGS,
    ADD_ATTR: DOMPURIFY_ATTR,
    HTML_INTEGRATION_POINTS: { foreignobject: true },
  });

  removeTempElements();

  return {
    diagramType: diagram.type,
    svg: svgCode,
    bindFunctions: diagram.db.bindFunctions,
  };
}

function initialize(userOptions = {}) {
  const options = assignWithDepth_default({}, userOptions);

  if (options?.fontFamily && !options.themeVariables?.fontFamily) {
    if (!options.themeVariables) {
      options.themeVariables = {};
    }
    options.themeVariables.fontFamily = options.fontFamily;
  }

  saveConfigFromInitialize(options);

  if (options?.theme && options.theme in themes_default) {
    options.themeVariables = themes_default[options.theme].getThemeVariables(options.themeVariables);
  } else {
    options.themeVariables = themes_default.default.getThemeVariables(options.themeVariables);
  }

  const config = typeof options === "object" ? setSiteConfig(options) : getConfig();
  setLogLevel(config.logLevel);
  addSupportedDiagrams();
}

setLogLevel(getConfig().logLevel);
reset(getConfig());

export default {
  initialize,
  render,
};
