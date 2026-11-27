// Runs one member and fails when it left the checkout different: `node scripts/write-nothing.ts
// [--root <dir>] -- <command> [args...]`. It lists every file under the root (the explorer by
// default) but node_modules, by path, size and modification time, runs the command in the folder it was started in with its
// output passed through, lists them again, and goes red naming each file the command added, changed or
// removed, so an ignored file counts too. The command's own exit code is kept when it fails.
//
// The explorer's members (typecheck, lint, test) write nothing under the checkout: cmd/gragas-new's
// copy test copies the tree and then walks the live one again while the members run beside it, so
// a file a member writes in between is a stray.
// The typecheck and the lint run through this script, in package.json and in the push's
// explorer-typecheck member (which runs tsc itself, once per package), so one run of each is the whole proof; the
// contract's build runs before it, as the step that places the dist the members read.
//
// It is a gate (ts.gates.1): the guard is imported first, a missing command is refused, and the
// exit code is set, never process.exit.
import "./gate-guard.ts";
import { program } from "./program.ts";
import { spawn } from "node:child_process";
import { lstatSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// snapshot lists every file under dir, but node_modules, as name -> "size mtime". A name is the
// entry names from the root down, joined with "/" on every system and never an absolute path, so a
// report reads the same on each; `prefix` is the names above dir, with a "/" after.
function snapshot(dir: string, prefix: string, out: Map<string, string>): void {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === "node_modules") continue;
    const path = join(dir, entry.name);
    if (entry.isDirectory()) snapshot(path, `${prefix}${entry.name}/`, out);
    else {
      const { size, mtimeMs } = lstatSync(path);
      out.set(`${prefix}${entry.name}`, `${String(size)} ${String(mtimeMs)}`);
    }
  }
}

// changes names each file that is in one snapshot and not the other, or differs in both.
function changes(
  before: Map<string, string>,
  after: Map<string, string>,
): string[] {
  const out: string[] = [];
  for (const [path, stat] of after) {
    const was = before.get(path);
    if (was === undefined) out.push(`added ${path}`);
    else if (was !== stat) out.push(`changed ${path}`);
  }
  for (const path of before.keys()) {
    if (!after.has(path)) out.push(`removed ${path}`);
  }
  return out.sort();
}

// run resolves with the command's exit code, its output going where this script's goes.
function run(cmd: string, args: string[], cwd: string): Promise<number> {
  return new Promise((resolveRun, reject) => {
    const [file, fileArgs] = program(cmd, args);
    const child = spawn(file, fileArgs, { cwd, stdio: "inherit" });
    child.on("error", reject);
    child.on("close", (code) => {
      resolveRun(code ?? 1);
    });
  });
}

async function main(): Promise<void> {
  const argv = process.argv.slice(2);
  let root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  if (argv[0] === "--root" && argv[1] !== undefined) {
    root = resolve(argv[1]);
    argv.splice(0, 2);
  }
  if (argv[0] === "--") argv.shift();
  const [cmd, ...args] = argv;
  if (cmd === undefined) {
    throw new Error(
      "usage: write-nothing.ts [--root <dir>] -- <command> [args...]",
    );
  }
  const before = new Map<string, string>();
  snapshot(root, "", before);
  const code = await run(cmd, args, process.cwd());
  const after = new Map<string, string>();
  snapshot(root, "", after);
  const written = changes(before, after);
  if (written.length > 0) {
    process.stderr.write(
      `write-nothing: ${cmd} ${args.join(" ")} wrote under ${root}:\n${written.map((line) => `  ${line}\n`).join("")}`,
    );
  }
  process.exitCode = code !== 0 ? code : written.length > 0 ? 1 : 0;
}

await main();
