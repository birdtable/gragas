// The banner: a notice on a page. Its tone is a word and an icon together, never
// a colour alone; an error is announced at once (alert), the rest politely.
import { Button } from "@base-ui/react/button";
import type { ReactElement, ReactNode } from "react";
import { Icon, type IconName } from "../icons.tsx";
import { say } from "../say.ts";
import "../base.css";
import "./banner.css";

export const tones = ["info", "success", "warning", "error"] as const;
export type Tone = (typeof tones)[number];

export const states = [...tones, "dismissible"] as const;

const toneIcon: Readonly<Record<Tone, IconName>> = {
  info: "info",
  success: "check",
  warning: "warning",
  error: "warning",
};

export type BannerProps = Readonly<{
  tone: Tone;
  children: ReactNode;
  onDismiss?: () => void;
}>;

export function Banner({
  tone,
  children,
  onDismiss,
}: BannerProps): ReactElement {
  return (
    <div
      data-part="banner"
      data-tone={tone}
      role={tone === "error" ? "alert" : "status"}
    >
      <Icon name={toneIcon[tone]} />
      <span className="gr-banner-tone">{say(`part.banner.${tone}`)}</span>
      <span className="gr-banner-body">{children}</span>
      {onDismiss === undefined ? null : (
        <Button
          className="gr-button gr-target"
          aria-label={say("part.dismiss")}
          onClick={onDismiss}
        >
          <Icon name="close" />
        </Button>
      )}
    </div>
  );
}
