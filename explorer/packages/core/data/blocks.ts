// The real reader of the page's JSON script blocks (the port
// load.ts declares as InlineBlocks). The standalone packaging writes each inlined
// document into `<script type="application/json" data-file="<file>">` (never an
// id: an id can collide with the page's own); the block is data, never executed,
// so reading it is reading text. A page with no such
// block answers undefined and the loader fetches instead. The node tests pass a
// fake of this port; the browser tests run this one against a real document.
import type { DocumentFile, InlineBlocks } from "./load.ts";

export function documentBlocks(page: Document): InlineBlocks {
  return {
    read(file: DocumentFile): string | undefined {
      for (const block of page.querySelectorAll(
        'script[type="application/json"]',
      )) {
        if (block.getAttribute("data-file") === file) return block.textContent;
      }
      return undefined;
    },
  };
}
