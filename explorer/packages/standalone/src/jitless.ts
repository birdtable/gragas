// The page runs under trusted types (web.speed.4): zod's probe for
// code generation calls the Function constructor, which the policy reports as a
// violation even though the throw is caught. Imported first by main.tsx, so the
// switch is set before any schema parses.
import * as z from "zod";

z.config({ jitless: true });
