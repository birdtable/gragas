// The shape of a part's stories. Storybook reads a story file by
// its structure, so a story file does not need Storybook's own types, and those
// do not pass the compiler under the strictest flags (exactOptionalPropertyTypes
// with skipLibCheck off), so no file that is type-checked imports them.
import type { ReactElement } from "react";

/** An engine rule a part's stories are excused from, and why; the list only shrinks. */
export type KnownFinding = Readonly<{ rule: string; reason: string }>;

export type Meta<P> = Readonly<{
  title: string;
  /** The part's states, from its module's `states`: each has a story named in PascalCase. */
  states: readonly string[];
  component: (props: P) => ReactElement;
  /** The accessibility state of the part's stories (seeded empty). */
  parameters: Readonly<{ a11y: Readonly<{ known: readonly KnownFinding[] }> }>;
}>;

/** One state of a part: the props it is drawn with. */
export type Story<P> = Readonly<{ args: P }>;
