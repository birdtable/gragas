// What to spawn for a command a gate script runs. A bare pnpm is looked up on the path, and the
// copy found there may be a wrapper the spawn does not run, so pnpm runs from the path it was
// started from (npm_execpath, set in every script pnpm runs): as node and that script when it is
// a script, as the lint and typecheck members run eslint and tsc, and as that program when it is the pinned binary.
// Anything else is spawned as named.
import { basename } from "node:path";

export function program(cmd: string, args: string[]): [string, string[]] {
  const entry = process.env["npm_execpath"];
  if (cmd === "pnpm" && entry !== undefined && /pnpm/i.test(basename(entry))) {
    return /\.[cm]?js$/i.test(entry)
      ? [process.execPath, [entry, ...args]]
      : [entry, args];
  }
  return [cmd, args];
}
