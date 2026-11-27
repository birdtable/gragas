// The funnel under Trusted Types: a page that requires them takes
// the funnel's output and refuses every other write of a string as markup. The
// requirement is a CSP meta element added before anything renders; this file
// runs in a page of its own, so it reaches no other test.
import { beforeAll, expect, test } from "vitest";
import { renderInto } from "../packages/core/markup/index.ts";

beforeAll(() => {
  const meta = document.createElement("meta");
  meta.httpEquiv = "Content-Security-Policy";
  meta.content = "require-trusted-types-for 'script'";
  document.head.append(meta);
});

test("the engine enforces Trusted Types once the page requires them", () => {
  expect(window.trustedTypes).toBeDefined();
  const el = document.createElement("div");
  expect(() => {
    el.innerHTML = "<b>raw</b>";
  }).toThrow(TypeError);
  expect(() => {
    el.insertAdjacentHTML("beforeend", "<b>raw</b>");
  }).toThrow(TypeError);
  expect(() => {
    el.setHTMLUnsafe("<b>raw</b>");
  }).toThrow(TypeError);
});

test("the funnel still renders, through its own policy", () => {
  const el = document.createElement("div");
  document.body.append(el);
  renderInto(el, "**bold** and [a](https://example.test/)");
  expect(el.querySelector("strong")?.textContent).toBe("bold");
  expect(el.querySelector("a")?.getAttribute("href")).toBe(
    "https://example.test/",
  );
  el.remove();
});

test("a string that did not come from the funnel is no TrustedHTML", () => {
  const el = document.createElement("div");
  expect(() => {
    Reflect.set(el, "innerHTML", "<img src=x onerror=1>");
  }).toThrow(TypeError);
  expect(el.children).toHaveLength(0);
});
