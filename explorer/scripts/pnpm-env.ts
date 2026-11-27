// The environment for a pnpm that reads the installed workspace at root, and so
// must write nothing in it. Given no store, pnpm creates and
// removes a probe file in the folder it runs in, to choose a store on that
// folder's file system, and that moves the folder's modification time. Go's
// test cache hashes the time of every entry of each folder a test lists, and
// the Go tests that walk the checkout list the repository root, so each run of
// the licence listing made the next push run those packages afresh. The store
// named here is the one the install recorded in node_modules/.modules.yaml;
// licenses list reads node_modules and not the store, so the name changes no
// listing, only stops the probe. A workspace with no install (a plant's
// throwaway one) has nothing to name, and its environment is left as it is.
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import * as z from "zod";

const ModulesSchema = z.object({ storeDir: z.string().min(1) });

export function pnpmEnv(root: string): NodeJS.ProcessEnv {
  const file = join(root, "node_modules", ".modules.yaml");
  if (!existsSync(file)) return process.env;
  const { storeDir } = ModulesSchema.parse(
    JSON.parse(readFileSync(file, "utf8")),
  );
  return { ...process.env, pnpm_config_store_dir: storeDir };
}
