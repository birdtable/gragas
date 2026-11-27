// Written by gragas-generate from internal/host/assets/contract/schema/v1/common.schema.json; edits are lost, so edit the source and run task gen.

import { z } from "zod";

/** the contract version a document is written in: the major is this folder's, the minor is the document's; a reader that finds a newer major stops and names both versions. */
export const Contract = z.string().regex(new RegExp("^1\\.(0|[1-9][0-9]*)$"));
export type Contract = z.infer<typeof Contract>;

export const Version = z.string().regex(new RegExp("^[0-9]+\\.[0-9]+(\\.[0-9]+)?$"));
export type Version = z.infer<typeof Version>;

export const Semver = z.string().regex(new RegExp("^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)$"));
export type Semver = z.infer<typeof Semver>;

/** the three versions every report and bundle carries. */
export const Versions = z.strictObject({
  contract: Contract,
  format: z.number().int().min(1),
  engine: Semver,
});
export type Versions = z.infer<typeof Versions>;

export const Id = z.string().regex(new RegExp("^[a-z]+\\.[a-z0-9]+(-[a-z0-9]+)*\\.[1-9][0-9]*$"));
export type Id = z.infer<typeof Id>;

export const Slug = z.string().regex(new RegExp("^[a-z0-9]+(-[a-z0-9]+)*$"));
export type Slug = z.infer<typeof Slug>;

export const Text = z.string().min(1);
export type Text = z.infer<typeof Text>;

export const Date = z.string().regex(new RegExp("^[0-9]{4}-[0-9]{2}-[0-9]{2}$"));
export type Date = z.infer<typeof Date>;

/** UTC, to the second. */
export const Timestamp = z.string().regex(new RegExp("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"));
export type Timestamp = z.infer<typeof Timestamp>;

export const Digest = z.string().regex(new RegExp("^sha256:[0-9a-f]{64}$"));
export type Digest = z.infer<typeof Digest>;

export const Language = z.string().regex(new RegExp("^[a-z]{2,3}(-[A-Za-z0-9]+)*$"));
export type Language = z.infer<typeof Language>;

export const Url = z.string().regex(new RegExp("^https://[^\\s]+$"));
export type Url = z.infer<typeof Url>;

/** plain words, display only. */
export const Reason = z.string().min(1);
export type Reason = z.infer<typeof Reason>;

export const Strength = z.enum(["must", "should", "may"]);
export type Strength = z.infer<typeof Strength>;

export const Bump = z.enum(["major", "minor", "patch"]);
export type Bump = z.infer<typeof Bump>;

export const Status = z.enum(["proposed", "trial", "active", "retired"]);
export type Status = z.infer<typeof Status>;

export const Origin = z.enum(["authored", "imported", "migrated"]);
export type Origin = z.infer<typeof Origin>;

export const Rung = z.number().int().min(0).max(6);
export type Rung = z.infer<typeof Rung>;

export const RunPoint = z.enum(["edit", "commit", "push", "land", "review", "release", "deploy"]);
export type RunPoint = z.infer<typeof RunPoint>;

export const TargetType = z.enum(["source", "config", "migration", "container", "workflow", "iac", "api-spec", "prose", "commit", "telemetry-registry", "dependency", "package"]);
export type TargetType = z.infer<typeof TargetType>;

export const Severity = z.enum(["error", "warning", "info"]);
export type Severity = z.infer<typeof Severity>;

/** derived from status, bindings and proof; a document carries it, no person types it. */
export const Standing = z.enum(["proposed", "enforced", "reviewed", "planned", "advisory", "retired"]);
export type Standing = z.infer<typeof Standing>;

/** a mark beside a standing, derived like it and never replacing it (S32). */
export const StandingMark = z.enum(["trial", "blocked", "unproven"]);
export type StandingMark = z.infer<typeof StandingMark>;

/** the window of a deprecated shape: x-deprecated on a field carries this. */
export const Deprecation = z.strictObject({
  since: Contract,
  removed_in: z.string().regex(new RegExp("^[2-9][0-9]*\\.0$")),
});
export type Deprecation = z.infer<typeof Deprecation>;

export const Mark = z.enum(["same", "tighter", "weaker", "none"]);
export type Mark = z.infer<typeof Mark>;

export const Result = z.enum(["pass", "fail", "not run", "reviewed"]);
export type Result = z.infer<typeof Result>;

/** text whose absence means empty: the one text shape that takes the empty string. */
export const TextOrEmpty = z.string();
export type TextOrEmpty = z.infer<typeof TextOrEmpty>;

/** a digest whose absence means empty: no digest. */
export const DigestOrEmpty = z.string().regex(new RegExp("^(sha256:[0-9a-f]{64})?$"));
export type DigestOrEmpty = z.infer<typeof DigestOrEmpty>;
