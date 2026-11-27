// A destination, a parameter and a search key are checked where a link is
// written (react.routing.2): a link with a key the route's schema
// does not name fails the type check. The plant is a file in memory, added to
// the package's own program; nothing is written to the tree.
import { fileURLToPath } from "node:url";
import { dirname, isAbsolute, join, relative, resolve } from "node:path";
import ts from "typescript";
import { expect, test } from "vitest";

const here = fileURLToPath(new URL("..", import.meta.url));
const src = join(here, "src");
const plantPath = join(src, "plant-link.tsx");

// tsc names a file in its own form, which need not be the one join gave the
// plant: resolve puts both in the system's form before they are compared.
const isPlant = (f: string): boolean => resolve(f) === resolve(plantPath);
const inSrc = (f: string): boolean => {
  const r = relative(src, resolve(f));
  return r !== "" && !r.startsWith("..") && !isAbsolute(r);
};

function check(body: string): string[] {
  const configPath = join(here, "tsconfig.json");
  const read = ts.readConfigFile(configPath, ts.sys.readFile);
  const parsed = ts.parseJsonConfigFileContent(
    read.config,
    ts.sys,
    dirname(configPath),
  );
  const host = ts.createCompilerHost(parsed.options);
  const code = `import { Link } from "@tanstack/react-router";\nexport function Plant(): unknown {\n  return ${body};\n}\n`;
  const realRead = host.readFile.bind(host);
  const realExists = host.fileExists.bind(host);
  const realSource = host.getSourceFile.bind(host);
  host.readFile = (f) => (isPlant(f) ? code : realRead(f));
  host.fileExists = (f) => isPlant(f) || realExists(f);
  host.getSourceFile = (f, lang, onError, update) =>
    isPlant(f)
      ? ts.createSourceFile(f, code, lang)
      : realSource(f, lang, onError, update);
  const program = ts.createProgram(
    [...parsed.fileNames.filter(inSrc), plantPath],
    parsed.options,
    host,
  );
  const sourceFile = program.getSourceFile(plantPath);
  // A plant the program does not hold is the harness's fault, never a finding:
  // it throws, so no case passes on it.
  if (sourceFile === undefined || sourceFile.text !== code) {
    throw new Error(`the plant is not in the program: ${plantPath}`);
  }
  return ts
    .getPreEmitDiagnostics(program, sourceFile)
    .map((d) => ts.flattenDiagnosticMessageText(d.messageText, "\n"));
}

test("a link with a search key the route names passes (the control)", () => {
  expect(check('<Link to="/rules" search={{ q: "go" }}>x</Link>')).toEqual([]);
});

// Each plant fails on its own fault: one diagnostic, the one that names it.
test("a link with a search key the route does not name fails the type check (the plant)", () => {
  const found = check('<Link to="/rules" search={{ bogus: 1 }}>x</Link>');
  expect(found).toHaveLength(1);
  expect(found[0]).toMatch(/'bogus' does not exist in type/);
});

test("a link to an address no route has fails the type check", () => {
  const found = check('<Link to="/nowhere">x</Link>');
  expect(found).toHaveLength(1);
  expect(found[0]).toMatch(/Type '"\/nowhere"' is not assignable to type/);
});
