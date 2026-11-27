// ts.gates.1: a Node gate imports this first. It registers the handler for an
// uncaught exception and an unhandled rejection before any other module of the
// gate loads, so a throw while the gate loads or runs prints the gate's name and
// leaves a failing exit code. It never calls process.exit: the event loop
// drains, so every pending write is flushed.
import { basename } from "node:path";

const gate = basename(process.argv[1] ?? "a gate");

function fail(error: unknown): void {
  process.stderr.write(
    `${gate}: ${error instanceof Error ? error.message : String(error)}\n`,
  );
  process.exitCode = 1;
}

process.on("uncaughtException", fail);
process.on("unhandledRejection", fail);
