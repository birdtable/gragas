// The explorer's one lint config (ts.lint.3). Every rule is an error
// (ts.lint-config.2): the shared configs are lifted to error below, and
// test/lint-config.test.ts reads the resolved config and fails on any rule
// left as a warning. Typed lint is never cached across files (ts.lint-config.3):
// pnpm lint passes no --cache. The edit tier lints through the untyped config
// in eslint.untyped.config.js (ts.lint-config.4).
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import js from "@eslint/js";
import vitest from "@vitest/eslint-plugin";
import boundaries from "eslint-plugin-boundaries";
import jsxA11y from "eslint-plugin-jsx-a11y";
import react from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import zod from "eslint-plugin-zod";
import globals from "globals";
import tseslint from "typescript-eslint";

const repo = fileURLToPath(new URL("..", import.meta.url));
const explorer = fileURLToPath(new URL(".", import.meta.url));

// The boundary table is rendered from layers.yaml by gragas-dev layers: edit
// that, never the JSON, and never the rules below (arch.ports.8).
const table = JSON.parse(
  readFileSync(
    new URL("../docs/explorer-boundaries.json", import.meta.url),
    "utf8",
  ),
);

const errorsOnly = (config) => {
  const rules = {};
  for (const [name, entry] of Object.entries(config.rules ?? {})) {
    const [level, ...options] = Array.isArray(entry) ? entry : [entry];
    rules[name] =
      level === "off" || level === 0 ? entry : ["error", ...options];
  }
  return { ...config, rules };
};

const asErrors = (configs) =>
  configs.flatMap((c) => (Array.isArray(c) ? asErrors(c) : [errorsOnly(c)]));

// ts.runtime.2: the deprecated core modules.
const deprecatedModules = [
  "_http_agent",
  "_http_client",
  "_http_common",
  "_http_incoming",
  "_http_outgoing",
  "_http_server",
  "_stream_duplex",
  "_stream_passthrough",
  "_stream_readable",
  "_stream_transform",
  "_stream_wrap",
  "_stream_writable",
  "_tls_common",
  "_tls_wrap",
  "constants",
  "domain",
  "punycode",
  "sys",
  "tls_common",
].flatMap((m) => [m, `node:${m}`]);

// react.components.3, web.structure.2: HTML from data is written in one module
// (packages/core/markup), which is also the only importer of the sanitiser and
// the renderer. Everywhere else a raw markup sink or a direct import of either
// library is an error.
const funnelFiles = ["packages/core/markup/**/*.ts"];
const funnelLibraries = ["dompurify", "markdown-it"].map((name) => ({
  name,
  message: `Only packages/core/markup imports ${name} (react.components.3).`,
}));
const sinkNames = String.raw`/^(innerHTML|outerHTML)$/`;
const markupSinks = [
  {
    selector: `AssignmentExpression[left.property.name=${sinkNames}]`,
    message:
      "No raw markup write: render through core/markup (react.components.3).",
  },
  {
    selector: `Literal[value=${sinkNames}]`,
    message:
      "No raw markup write by name: render through core/markup (react.components.3).",
  },
  {
    selector:
      "CallExpression[callee.property.name=/^(insertAdjacentHTML|setHTMLUnsafe|parseHTMLUnsafe)$/]",
    message:
      "No raw markup write: render through core/markup (react.components.3).",
  },
  {
    selector:
      "CallExpression[callee.object.name='document'][callee.property.name=/^write(ln)?$/]",
    message:
      "No document.write: render through core/markup (react.components.3).",
  },
];
// react.components.4: a DOM element takes no style prop; the tokens style it.
const domStyleProp = {
  selector:
    "JSXOpeningElement[name.name=/^[a-z]/] > JSXAttribute[name.name='style']",
  message: "No style prop on a DOM element (react.components.4).",
};
const moduleSyntax = [
  {
    selector: "TSEnumDeclaration",
    message: "No enum (ts.modules.2): use a literal union or a schema enum.",
  },
  {
    selector: "TSModuleDeclaration[kind='namespace']",
    message: "No namespace (ts.modules.2).",
  },
  {
    selector: "TSModuleDeclaration[kind='module']",
    message: "No module block (ts.modules.2).",
  },
  {
    selector: "ExportDefaultDeclaration",
    message: "Named exports only (ts.modules.1).",
  },
  {
    selector:
      "ExportNamedDeclaration > ExportSpecifier[exported.name='default']",
    message: "Named exports only (ts.modules.1).",
  },
];
const relativeImports = {
  regex: String.raw`^\.{1,2}(/[^/]+)*/[^/.]+$`,
  message: "A relative import carries its extension (ts.modules.3).",
};
const deprecatedPaths = deprecatedModules.map((name) => ({
  name,
  message: "A deprecated core module (ts.runtime.2).",
}));
// The globals a module never names (ts.modules.3 and the CommonJS names).
const moduleGlobals = [
  "require",
  "module",
  "exports",
  "__dirname",
  "__filename",
];
// web.structure.1: one module touches the network or writes a URL, and it is
// packages/core/data/load.ts; every other source file of a package is refused
// these names, the tests excepted.
const networkGlobals = [
  "fetch",
  "Request",
  "XMLHttpRequest",
  "WebSocket",
  "EventSource",
  "URL",
  "URLSearchParams",
].map((name) => ({
  name,
  message:
    "Only packages/core/data/load.ts touches the network or writes a URL (web.structure.1).",
}));

// The explorer's TypeScript holds no knowledge of the operating system. Node's node:path and node:url are the portable calls, so
// the lint refuses the reads that name a system: process.platform and
// os.platform(), path.sep, path.posix and path.win32, a path built or split on
// a backslash or a joined slash, and the pathname of a file URL (use
// fileURLToPath). A system's path is read through node:url, never by hand.
const osProperties = [
  ["process", "platform"],
  ["os", "platform"],
  ["os", "EOL"],
  ["os", "type"],
  ["path", "sep"],
  ["path", "posix"],
  ["path", "win32"],
].map(([object, property]) => ({
  object,
  property,
  message: `No ${object}.${property}: the explorer knows no operating system.`,
}));
const osImports = [
  { name: "node:os", importNames: ["platform", "type", "EOL"] },
  { name: "node:path", importNames: ["sep", "posix", "win32"] },
  { name: "node:path/posix" },
  { name: "node:path/win32" },
].map((entry) => ({
  ...entry,
  message: "The explorer knows no operating system.",
}));
const osSyntax = [
  {
    selector:
      "CallExpression[callee.property.name=/^replace(All)?$/]:matches([arguments.0.value='\\\\'], [arguments.0.regex.pattern='\\\\\\\\'])",
    message:
      "No replace of a backslash: a system's path is not read by hand (no operating system).",
  },
  {
    selector: "Literal[value=/[.](cmd|exe)$|^\\/dev\\/null$/i]",
    message:
      "No program or device name of a system (.cmd, .exe, /dev/null): the explorer knows no operating system.",
  },
  {
    // A template literal reads by its quasis: `${x}.exe` ends in the quasi ".exe".
    selector: "TemplateElement[value.cooked=/[.](cmd|exe)$|^\\/dev\\/null$/i]",
    message:
      "No program or device name of a system (.cmd, .exe, /dev/null): the explorer knows no operating system.",
  },
  {
    selector:
      "CallExpression[callee.property.name='split'][arguments.0.value='\\\\']",
    message:
      "No split on a backslash: a system's path is not read by hand (no operating system).",
  },
  {
    selector:
      "CallExpression[callee.property.name='join']:matches([arguments.0.value='/'], [arguments.0.value='\\\\'])",
    message:
      "No join on a separator literal: use node:path join (no operating system).",
  },
  {
    selector:
      "MemberExpression[property.name='pathname'][object.type='NewExpression'][object.arguments.1.object.type='MetaProperty']",
    message:
      "No pathname of a file URL: use fileURLToPath of node:url (no operating system).",
  },
  {
    selector:
      "MemberExpression[property.name='pathname'][object.type='NewExpression'][object.arguments.0.object.type='MetaProperty']",
    message:
      "No pathname of a file URL: use fileURLToPath of node:url (no operating system).",
  },
];

const testFiles = [
  "**/*.test.ts",
  "**/*.test.tsx",
  "**/test/**/*.ts",
  "**/test/**/*.tsx",
];
const scriptFiles = [
  "*.js",
  "*.mjs",
  "*.cjs",
  "**/*.config.js",
  "**/*.config.ts",
  "count-reporter.ts",
  "scripts/**",
];
const configFiles = [
  "*.config.js",
  "**/*.config.ts",
  "**/*.config.js",
  "count-reporter.ts",
];

export default [
  {
    // ts.lint-config.8: build, coverage, cache and tool output.
    ignores: [
      "**/dist/**",
      "**/coverage/**",
      "**/.cache/**",
      "**/.tsbuildinfo",
      "**/node_modules/**",
      "**/*.gen.ts",
    ],
  },
  {
    linterOptions: {
      reportUnusedDisableDirectives: "error",
      noInlineConfig: false,
    },
  },
  ...asErrors([
    js.configs.recommended,
    ...tseslint.configs.strictTypeChecked,
    ...tseslint.configs.stylisticTypeChecked,
  ]),
  {
    files: ["**/*.ts", "**/*.tsx"],
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: explorer },
    },
    rules: {
      // ts.type-safety.1, .2, .3, .10
      "@typescript-eslint/no-explicit-any": "error",
      "@typescript-eslint/no-non-null-assertion": "error",
      "@typescript-eslint/consistent-type-assertions": [
        "error",
        { assertionStyle: "never" },
      ],
      "@typescript-eslint/ban-ts-comment": [
        "error",
        {
          "ts-expect-error": true,
          "ts-ignore": true,
          "ts-nocheck": true,
          "ts-check": false,
        },
      ],
      "@typescript-eslint/explicit-module-boundary-types": "error",
      "@typescript-eslint/explicit-function-return-type": [
        "error",
        { allowExpressions: true, allowTypedFunctionExpressions: true },
      ],
      // ts.lint.1
      "@typescript-eslint/no-floating-promises": "error",
      "@typescript-eslint/no-misused-promises": "error",
      "@typescript-eslint/no-unsafe-assignment": "error",
      "@typescript-eslint/no-unsafe-call": "error",
      "@typescript-eslint/no-unsafe-member-access": "error",
      "@typescript-eslint/no-unsafe-return": "error",
      "@typescript-eslint/no-unsafe-argument": "error",
      // ts.errors.1, .4
      "no-empty": ["error", { allowEmptyCatch: false }],
      "@typescript-eslint/no-empty-function": "error",
      "no-console": "error",
      // ts.modules.1, .2
      "no-restricted-syntax": [
        "error",
        ...moduleSyntax,
        ...markupSinks,
        domStyleProp,
        ...osSyntax,
      ],
      "no-restricted-properties": ["error", ...osProperties],
      // ts.modules.3 (a relative import carries its extension), ts.runtime.2,
      // and react.components.3 (the funnel's two libraries)
      "no-restricted-imports": [
        "error",
        {
          paths: [...deprecatedPaths, ...funnelLibraries, ...osImports],
          patterns: [relativeImports],
        },
      ],
      "no-restricted-globals": ["error", ...moduleGlobals],
    },
  },
  // react.lint.1, .2, .3
  {
    files: ["**/*.tsx"],
    plugins: { react, "react-hooks": reactHooks, "jsx-a11y": jsxA11y },
    settings: { react: { version: "19.1" } },
    rules: {
      ...react.configs.flat.recommended.rules,
      ...react.configs["jsx-runtime"].rules,
      ...reactHooks.configs.flat.recommended.rules,
      ...jsxA11y.flatConfigs.strict.rules,
      "react/jsx-key": "error",
      "react/no-unstable-nested-components": "error",
      "react/no-danger": "error",
    },
  },
  // The schema library's rules over the contract and wherever a schema is written.
  {
    files: ["packages/contract/**/*.ts"],
    ...errorsOnly(zod.configs.recommended),
  },
  // web.structure.1: the network and the URL belong to one module.
  {
    files: ["packages/**/*.ts", "packages/**/*.tsx"],
    ignores: [
      "packages/core/data/load.ts",
      "packages/contract/src/fixtures/**",
      ...testFiles,
    ],
    rules: {
      "no-restricted-globals": ["error", ...moduleGlobals, ...networkGlobals],
      "no-restricted-properties": [
        "error",
        ...osProperties,
        ...["fetch", "XMLHttpRequest", "WebSocket", "EventSource"].flatMap(
          (property) =>
            ["window", "globalThis", "self"].map((object) => ({
              object,
              property,
              message:
                "Only packages/core/data/load.ts touches the network (web.structure.1).",
            })),
        ),
      ],
    },
  },
  // arch.ports.8: the import boundaries between the explorer's modules.
  {
    files: ["packages/**/*.ts", "packages/**/*.tsx"],
    plugins: { boundaries },
    settings: {
      "boundaries/root-path": repo,
      "boundaries/elements": table.elements,
    },
    rules: {
      "boundaries/dependencies": [
        "error",
        {
          default: table.default,
          policies: table.rules.map((r) => ({
            from: { element: { type: r.from } },
            allow: {
              to: { element: { types: { anyOf: [r.from, ...r.allow] } } },
            },
          })),
        },
      ],
    },
  },
  // ts.lint-config.5: tests are linted without type information.
  {
    files: testFiles,
    ...tseslint.configs.disableTypeChecked,
    plugins: { vitest },
    rules: {
      ...tseslint.configs.disableTypeChecked.rules,
      ...errorsOnly(vitest.configs.recommended).rules,
    },
  },
  // ts.lint-config.6, .7: plain scripts and the root tool config files take the runtime's globals and no type information.
  {
    files: scriptFiles,
    ...tseslint.configs.disableTypeChecked,
    languageOptions: {
      ...tseslint.configs.disableTypeChecked.languageOptions,
      globals: globals.node,
    },
    rules: {
      ...tseslint.configs.disableTypeChecked.rules,
      "no-console": "off",
    },
  },
  // react.components.3: the funnel imports the two libraries and writes the one
  // sink; the browser tests prove the page refuses every other write, so they
  // call the sinks themselves. Both keep every other rule above.
  {
    files: funnelFiles,
    rules: {
      "no-restricted-syntax": ["error", ...moduleSyntax, ...osSyntax],
      "no-restricted-imports": [
        "error",
        {
          paths: [...deprecatedPaths, ...osImports],
          patterns: [relativeImports],
        },
      ],
    },
  },
  {
    files: ["browser/**/*.test.ts"],
    rules: {
      "no-restricted-syntax": [
        "error",
        ...moduleSyntax,
        domStyleProp,
        ...osSyntax,
      ],
    },
  },
  // ts.modules.1: the listed files that may default-export, because their tool reads the default.
  {
    files: funnelFiles,
    rules: {
      "no-restricted-syntax": ["error", ...moduleSyntax, ...osSyntax],
      "no-restricted-imports": [
        "error",
        {
          paths: [...deprecatedPaths, ...osImports],
          patterns: [relativeImports],
        },
      ],
    },
  },
  {
    files: ["browser/**/*.test.ts"],
    rules: {
      "no-restricted-syntax": [
        "error",
        ...moduleSyntax,
        domStyleProp,
        ...osSyntax,
      ],
    },
  },
  // ts.modules.2: an ambient declaration file is where a module block belongs
  // (a stylesheet import, one library function); it keeps every other rule.
  {
    files: ["**/*.d.ts"],
    rules: {
      "no-restricted-syntax": [
        "error",
        ...moduleSyntax.filter((rule) => !rule.selector.includes("'module'")),
        ...osSyntax,
      ],
    },
  },
  // ts.modules.1: the listed files that may default-export, because their tool reads the default.
  // Storybook reads the default of a story file (its meta) and of its two
  // config files.
  {
    files: [...configFiles, "**/*.stories.tsx", "**/.storybook/*.ts"],
    rules: { "no-restricted-syntax": ["error", ...osSyntax] },
  },
];
