// What the bundler adds to the page's types: the build's names on import.meta
// and the style sheets the parts and the shell import for their effect.
interface ImportMeta {
  readonly env: Readonly<Record<string, unknown>>;
}

declare module "*.css";

declare module "*?raw" {
  const text: string;
  export = text;
}
