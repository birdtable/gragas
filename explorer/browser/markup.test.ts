// The Markdown funnel in real engines: every item of a hostile
// corpus renders inert, and a checked-in seed list of less-trusted prose is
// rendered under the same invariants (tests.generated.1). The checks are the
// test's own, not the funnel's: an allow list of tags, no event attribute, no
// style, no id, and no link or image source outside the allowed schemes. The
// corpus' payloads each try to set window.__pwned, which none may reach.
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { renderInto, renderMarkdown } from "../packages/core/markup/index.ts";

const hostile = import.meta.glob("./markup/hostile/*.md", {
  query: "?raw",
  import: "default",
  eager: true,
});
const seeds = import.meta.glob("./markup/seeds/*.md", {
  query: "?raw",
  import: "default",
  eager: true,
});

const allowedTags = new Set([
  "A",
  "BLOCKQUOTE",
  "BR",
  "CODE",
  "DEL",
  "EM",
  "H1",
  "H2",
  "H3",
  "H4",
  "H5",
  "H6",
  "HR",
  "IMG",
  "LI",
  "OL",
  "P",
  "PRE",
  "S",
  "STRONG",
  "TABLE",
  "TBODY",
  "TD",
  "TH",
  "THEAD",
  "TR",
  "UL",
]);
const allowedAttributes = new Set([
  "alt",
  "href",
  "rel",
  "src",
  "start",
  "title",
]);

function problems(root: ParentNode): string[] {
  const found: string[] = [];
  for (const el of root.querySelectorAll("*")) {
    if (!allowedTags.has(el.tagName)) found.push(`element ${el.tagName}`);
    for (const attr of el.attributes) {
      if (!allowedAttributes.has(attr.name)) {
        found.push(`attribute ${attr.name} on ${el.tagName}`);
      }
      if (attr.name.startsWith("on")) found.push(`event ${attr.name}`);
    }
    const href = el.getAttribute("href");
    if (href !== null && !/^(?:https?:|mailto:|#)/i.test(href)) {
      found.push(`href ${href}`);
    }
    const src = el.getAttribute("src");
    if (src !== null && !/^https?:/i.test(src)) found.push(`src ${src}`);
  }
  return found;
}

function names(files: Record<string, unknown>): string[] {
  return Object.keys(files).sort();
}

function pwned(): unknown {
  return Reflect.get(window, "__pwned");
}

let host: HTMLElement;

beforeEach(() => {
  Reflect.deleteProperty(window, "__pwned");
  host = document.createElement("div");
  document.body.append(host);
});

afterEach(() => {
  host.remove();
});

describe("the hostile corpus renders inert", () => {
  test("the corpus is there", () => {
    expect(names(hostile).length).toBeGreaterThanOrEqual(40);
  });

  test.each(names(hostile))("%s", async (name) => {
    const source = hostile[name];
    expect(typeof source).toBe("string");
    renderInto(host, String(source));
    expect(problems(host)).toEqual([]);
    // An image error or a focus handler fires after a tick, if it fires at all.
    await new Promise((done) => setTimeout(done, 150));
    expect(pwned()).toBeUndefined();
  });

  test("inside a shadow root, with no id or name to collide with the page", async () => {
    const page = document.createElement("a");
    page.id = "documentElement";
    document.body.append(page);
    const shadow = host.attachShadow({ mode: "open" });
    for (const name of names(hostile)) {
      renderInto(shadow, String(hostile[name]));
      expect(problems(shadow)).toEqual([]);
      expect(shadow.querySelector("[id], [name], [class], [style]")).toBeNull();
    }
    expect(document.getElementById("documentElement")).toBe(page);
    expect(document.documentElement.tagName).toBe("HTML");
    page.remove();
    await new Promise((done) => setTimeout(done, 100));
    expect(pwned()).toBeUndefined();
  });
});

describe("what the allow list keeps", () => {
  test("formatting, lists, code, a table and the allowed links survive", () => {
    renderInto(
      host,
      [
        "# Title",
        "",
        "**strong** *em* `code` ~~gone~~",
        "",
        "- one",
        "- two",
        "",
        "| a | b |",
        "|---|---|",
        "| 1 | 2 |",
        "",
        "[docs](https://example.test/docs) [mail](mailto:a@example.test) [here](#top)",
        "",
        "![alt text](https://example.test/a.png)",
      ].join("\n"),
    );
    for (const selector of [
      "h1",
      "strong",
      "em",
      "code",
      "s",
      "ul > li",
      "table th",
      "table td",
      "img[alt='alt text']",
    ]) {
      expect([selector, host.querySelector(selector) !== null]).toEqual([
        selector,
        true,
      ]);
    }
    const hrefs = [...host.querySelectorAll("a")].map((a) =>
      a.getAttribute("href"),
    );
    expect(hrefs).toEqual([
      "https://example.test/docs",
      "mailto:a@example.test",
      "#top",
    ]);
  });

  test("a link keeps its name and opens without the opener", () => {
    renderInto(host, "[the rule's page](https://example.test/rule)");
    const link = host.querySelector("a");
    expect(link?.textContent).toBe("the rule's page");
    expect(link?.getAttribute("rel")).toBe("noopener noreferrer");
  });

  test("raw HTML in the prose is text, never markup", () => {
    renderInto(host, "<b>not bold</b> and <script>1</script>");
    expect(host.querySelector("b")).toBeNull();
    expect(host.querySelector("script")).toBeNull();
    expect(host.textContent).toContain("<b>not bold</b>");
  });

  test("a relative link and a data image lose their target", () => {
    renderInto(host, "[page](../page.md) ![x](data:image/gif;base64,R0lGOD)");
    expect(host.querySelector("a")?.hasAttribute("href")).toBe(false);
    expect(host.querySelector("img")).toBeNull();
  });
});

describe("the seed list", () => {
  test("the seeds are there", () => {
    expect(names(seeds).length).toBeGreaterThanOrEqual(25);
  });

  test.each(names(seeds))("%s renders under every invariant", async (name) => {
    renderInto(host, String(seeds[name]));
    expect(problems(host)).toEqual([]);
    await new Promise((done) => setTimeout(done, 20));
    expect(pwned()).toBeUndefined();
  });

  test("rendering the same text twice gives the same markup", () => {
    for (const name of names(seeds)) {
      const source = String(seeds[name]);
      expect(String(renderMarkdown(source))).toBe(
        String(renderMarkdown(source)),
      );
    }
  });
});
