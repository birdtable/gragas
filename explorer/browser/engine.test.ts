import { expect, test } from "vitest";
import { commands } from "vitest/browser";

// A real element in a real engine: the browser project has no DOM shim, so
// layout is the engine's own.
test("a block element is laid out by the engine, not shimmed", () => {
  const el = document.createElement("div");
  el.style.cssText = "width: 120px; height: 40px";
  document.body.append(el);
  const box = el.getBoundingClientRect();
  expect(box.width).toBe(120);
  expect(box.height).toBe(40);
  el.remove();
});

test("the page is a real document with no network", async () => {
  expect(typeof window.matchMedia).toBe("function");
  const port = await commands.freePort();
  await expect(fetch(`http://127.0.0.1:${String(port)}/`)).rejects.toThrow();
});
