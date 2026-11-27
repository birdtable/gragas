// The one module that writes HTML from data (react.components.3,
// web.structure.2). Guide prose and reasoning come from a rulebook the engine
// does not control, so they are data: markdown-it renders them with raw HTML
// off, DOMPurify strips what is left to a short allow list, and the result goes
// to the page through a Trusted Types policy that only this module creates.
// This is the only file that imports dompurify or markdown-it, and the only
// one that assigns to a markup sink; lint refuses both elsewhere.
import DOMPurify from "dompurify";
import MarkdownIt from "markdown-it";

// Links keep http, https, mailto and in-page anchors; nothing else, so no
// javascript:, data:, vbscript: or blob: reaches an attribute.
const linkPattern = /^(?:https?:|mailto:|#)/i;
// An image keeps only an http or https source: no data: image, no anchor.
const imagePattern = /^https?:/i;

const allowedTags = [
  "a",
  "blockquote",
  "br",
  "code",
  "del",
  "em",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "hr",
  "img",
  "li",
  "ol",
  "p",
  "pre",
  "s",
  "strong",
  "table",
  "tbody",
  "td",
  "th",
  "thead",
  "tr",
  "ul",
];

// No id, name, class or style: the output cannot collide with the host page's
// ids or names, and carries no style of its own (react.components.4); it takes
// the surrounding element's styles, in a shadow root or out of one.
const allowedAttributes = ["alt", "href", "rel", "src", "start", "title"];

const markdown = new MarkdownIt({
  html: false,
  linkify: false,
  typographer: false,
});

const purifier = DOMPurify(window);

purifier.addHook("afterSanitizeAttributes", (node) => {
  if (node.tagName === "A") {
    const href = node.getAttribute("href");
    if (href !== null && !linkPattern.test(href.trim())) {
      node.removeAttribute("href");
    }
    node.setAttribute("rel", "noopener noreferrer");
  }
  if (node.tagName === "IMG") {
    const src = node.getAttribute("src");
    if (src === null || !imagePattern.test(src.trim())) {
      node.remove();
    }
  }
});

function sanitise(html: string): TrustedHTML | string {
  return purifier.sanitize(html, {
    ALLOWED_TAGS: allowedTags,
    ALLOWED_ATTR: allowedAttributes,
    ALLOWED_URI_REGEXP: linkPattern,
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false,
    ALLOW_SELF_CLOSE_IN_ATTR: false,
    KEEP_CONTENT: true,
    // Under Trusted Types the instance makes its one policy, named dompurify,
    // and keeps it to itself: nothing outside this module holds it. Without
    // Trusted Types the result is a string.
    RETURN_TRUSTED_TYPE: true,
  });
}

/** The sanitised HTML of a Markdown string, as the page may take it. */
export function renderMarkdown(source: string): TrustedHTML | string {
  return sanitise(markdown.render(source));
}

/** Replace a host's content with the rendered Markdown: the one markup sink. */
export function renderInto(host: Element | ShadowRoot, source: string): void {
  // lib.dom types the setter as string only, and a string would not pass a
  // page that enforces Trusted Types; Reflect hands the TrustedHTML object over
  // as it is. This line is the one write of markup in the explorer.
  Reflect.set(host, "innerHTML", renderMarkdown(source));
}
