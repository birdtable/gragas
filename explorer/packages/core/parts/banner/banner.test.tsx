// Every tone says its word and draws its icon; only an error is an alert.
import { userEvent } from "vitest/browser";
import { afterEach, describe, expect, test } from "vitest";
import { render, type Rendered } from "../../test/render.tsx";
import { Banner, tones } from "./banner.tsx";

let shown: Rendered | undefined;
afterEach(() => {
  shown?.unmount();
  shown = undefined;
});

describe("a tone", () => {
  for (const tone of tones) {
    test(`${tone} has a word and an icon`, () => {
      shown = render(<Banner tone={tone}>Body</Banner>);
      const part = shown.container.querySelector("[data-part='banner']");
      expect(part?.querySelector("svg")).not.toBeNull();
      expect(part?.querySelector(".gr-banner-tone")?.textContent).not.toBe("");
      expect(part?.getAttribute("role")).toBe(
        tone === "error" ? "alert" : "status",
      );
    });
  }
});

test("dismisses on click", () => {
  let dismissed = false;
  shown = render(
    <Banner
      tone="info"
      onDismiss={() => {
        dismissed = true;
      }}
    >
      Body
    </Banner>,
  );
  shown.container.querySelector("button")?.click();
  expect(dismissed).toBe(true);
});

test("dismisses by Enter and by Space", async () => {
  let dismissed = 0;
  shown = render(
    <Banner
      tone="info"
      onDismiss={() => {
        dismissed += 1;
      }}
    >
      Body
    </Banner>,
  );
  shown.container.querySelector("button")?.focus();
  await userEvent.keyboard("{Enter}");
  expect(dismissed).toBe(1);
  await userEvent.keyboard(" ");
  expect(dismissed).toBe(2);
});
