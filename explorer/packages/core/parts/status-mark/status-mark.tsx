// The status mark (spec 17): a standing, a status or an observation is always
// its word and its symbol together, with the colour role added on top, so
// nothing is told by colour alone (web.accessibility.4). A mark (trial, blocked,
// unproven) sits beside a standing as a hollow, an outlined or a dotted chip
// that carries its own word and symbol.
import type { ReactElement } from "react";
import { marks, observations, standings, statuses } from "@gragas/tokens";
import {
  markSymbols,
  observationSymbols,
  statusSymbols,
} from "../../messages/index.ts";
import { say } from "../say.ts";
import "../base.css";
import "./status-mark.css";

export type Standing = (typeof standings)[number]["standing"];
export type Status = (typeof statuses)[number]["status"];
export type Observation = (typeof observations)[number]["observation"];
export type Mark = (typeof marks)[number]["mark"];

/** Every state the catalogue holds a story for: each standing, each mark, a status, an observation. */
export const states = [
  ...standings.map((s) => s.standing),
  ...marks.map((m) => m.mark),
  "status",
  "observation",
] as const;

export type StatusMarkProps = Readonly<
  | { standing: Standing; mark?: Mark }
  | { status: Status }
  | { observation: Observation }
>;

const standingSymbol = (standing: Standing): string =>
  standings.find((s) => s.standing === standing)?.symbol ?? "";
const standingRole = (standing: Standing): string =>
  standings.find((s) => s.standing === standing)?.role ?? "grey";

function Chip({
  symbol,
  word,
}: {
  readonly symbol: string;
  readonly word: string;
}): ReactElement {
  return (
    <>
      <span className="gr-status-symbol" aria-hidden="true">
        {symbol}
      </span>
      <span className="gr-status-word">{word}</span>
    </>
  );
}

export function StatusMark(props: StatusMarkProps): ReactElement {
  if ("status" in props) {
    return (
      <span data-part="status-mark" data-status={props.status}>
        <Chip
          symbol={statusSymbols[props.status]}
          word={say(`status.${props.status}`)}
        />
      </span>
    );
  }
  if ("observation" in props) {
    return (
      <span data-part="status-mark" data-observation={props.observation}>
        <Chip
          symbol={observationSymbols[props.observation]}
          word={say(`observation.${props.observation}`)}
        />
      </span>
    );
  }
  const { standing, mark } = props;
  return (
    <span
      data-part="status-mark"
      data-standing={standing}
      data-role={standingRole(standing)}
    >
      <Chip
        symbol={standingSymbol(standing)}
        word={say(`standing.${standing}`)}
      />
      {mark === undefined ? null : (
        <>
          <span className="gr-hidden">, </span>
          <span className="gr-status-mark" data-mark={mark}>
            <Chip symbol={markSymbols[mark]} word={say(`mark.${mark}`)} />
          </span>
        </>
      )}
    </span>
  );
}
