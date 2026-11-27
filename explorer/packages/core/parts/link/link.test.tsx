// An external link opens a new tab with no opener, and says so.
import { userEvent } from "vitest/browser";
import { afterEach, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { Link } from "./link.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

test("an external link names the new tab and drops the opener", () => {
  shown = render(
    <Link href="https://example.org/" external>
      Example
    </Link>,
  );
  const a = shown.container.querySelector("a");
  expect(a?.target).toBe("_blank");
  expect(a?.rel).toContain("noopener");
  expect(a?.textContent).toContain("opens in a new tab");
});

test("an internal link stays in the tab", () => {
  shown = render(<Link href="#rules">The rules</Link>);
  const a = shown.container.querySelector("a");
  expect(a?.getAttribute("target")).toBeNull();
  expect(a?.textContent).toBe("The rules");
});

test("is followed by Enter", async () => {
  shown = render(<Link href="#rules">The rules</Link>);
  const a = shown.container.querySelector("a");
  let followed = 0;
  a?.addEventListener("click", (event) => {
    event.preventDefault();
    followed += 1;
  });
  // WebKit leaves links out of the Tab order by default, so focus it directly.
  a?.focus();
  expect(document.activeElement).toBe(a);
  await userEvent.keyboard("{Enter}");
  expect(followed).toBe(1);
});
