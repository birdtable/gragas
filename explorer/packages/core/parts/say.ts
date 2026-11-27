// The words a part says by default, from the message module (web.content.1): a
// part takes its words as props and falls back to these, so a host's override
// arrives through the page that places the part, not through a provider.
import { createSay, type Say } from "../messages/index.ts";

export const say: Say = createSay("en");
