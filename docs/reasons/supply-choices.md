---
type: reason
status: current
as-of: 2026-11-16
---

# Supply-chain rules and the choices behind them

The supply-chain part of the rulebook covers version bumps, installs, dependency intake, licences, provenance, releases, scans, secrets and workflows. The rules below share one basis: the project's target state is a local gate and a tag-driven release, with no hosted CI minutes spent on every push. This page gives the reason for each rule, in one place.

## The rules

- **supply.bumps.2** (Hard, active): A consumer that lags keeps running code the owner has already replaced, so the lag is shown. A fixed window forces every consumer to the newest version on the owner's schedule, which the architecture rules name as a cause of coupling; fixing callers in the breaking change is what keeps a lagging consumer safe to move later. A pair that must always release together signals a misplaced boundary.
- **supply.install.4** (Hard, active): A range on a direct dependency means two installs of one commit can differ. In the web baseline none of 32 direct dependencies was a range. Extending exact pins to the images a program deploys for its owner would forbid the update path that was chosen on purpose.
- **supply.install.8** (Hard, active): A package that resolves only because something else installed it breaks the day that other package changes, and a bare runner call can fetch a stranger. A root dependency hides the same phantom import inside a workspace.
- **supply.install.9** (Hard, active): A gate that ran on a different toolchain than the lockfile names proves nothing about the shipped build. One tool version per estate removes a class of drift.
- **supply.intake.2** (Hard, active): A module can be linked without anyone having chosen it. In the Go estate, direct requires ranged from 7 to 15 per repository and linked modules from 38 to 47, and no repository had an allow file. A count per repository hides a small binary that has grown the large one's dependencies, which is where a trust line breaks.
- **supply.intake.5** (Lens, active): A dependency or a helper for something the standard library already does is weight with no gain, and a part rebuilt where a maintained tool exists is code the project now maintains alone.
- **supply.licences.1** (Hard, active): A strong copyleft licence in a shipped dependency can force the product's source open, and an unlicensed one gives no right to use it. Weak copyleft binds only the files of the package itself, so it is safe while those files are not changed.
- **supply.licences.6** (Hard, active): Most permissive licences are conditional on keeping their notice; an artifact that bundles code without it breaks the licence that allowed the bundle. A hand-kept notices file drifts from what the artifact holds.
- **supply.provenance.7** (Hard, active): A program fetched outside the lockfile has no integrity check unless the step does one, and a step that skips a check it cannot run installs whatever the network handed it.
- **supply.release.1** (Hard, active): A shared package can otherwise publish from a laptop with whatever the laptop holds, and an install from an untagged tree is a version nobody can name. The invariant three target repositories share is one recipe from a clean tagged main after the full gate; CI is the only place that holds the publish token.
- **supply.release.4** (Hard, retired): A checksum lets a consumer detect corruption or substitution; heavier attestations cost work with no reader until a verifier exists.
- **supply.release.5** (Hard, active): A tag with no entry gives consumers nothing to read, and a generated entry made of commit subjects is written for the author, not the reader. A version a tool proposes is not mistyped, and an override keeps the person's choice. A recipe must not push releases without the person's last look.
- **supply.release.6** (Hard, active): A bundle can differ from the source the tests ran against through bundler options or an optional dependency resolved at build time. A last-modified time shows the function was updated, not that its bytes are the ones that passed. In one web estate 4 of 72 faults showed only on a real server after release.
- **supply.release.9** (Hard, active): A checksum lets a consumer detect corruption or substitution. Provenance, an SBOM and a pinned non-root base cost minutes once and age well; waiting for a third party to ask is waiting to be asked why not.
- **supply.scans.1** (Hard, active): A dependency with a known vulnerability is the cheapest way in for an attacker. Before this rule the web catalogue had no mention of vulnerabilities, and audit appeared only as a late-tier report. The Go scan was already in six of seven repositories. There is no hosted land job, so a scan placed only at land would have no tier that runs it.
- **supply.secrets.1** (Hard, active): A committed secret stays in history forever and must be rotated. The scan also reaches the web bundle, which must carry no secret. There is no hosted land job, so a full scan placed only at land would never run.
- **supply.secrets.4** (Hard, active): A private name in history cannot be removed without rewriting it, and a public repository publishes every word of it. In one target repository a work email reached 217 commits before the check existed. A list kept in the repo would itself publish the names.
- **supply.workflows.1** (Hard, active): The project does not spend hosted minutes on every push: the tag is the release and the local gate is the gate. Neither the workflow linter nor the security audit checks the trigger set, so a push trigger went unseen.
- **supply.workflows.5** (Hard, active): Hosted minutes on every push are not spent, and a workflow with its own gate steps drifts from the gate the hook runs, so the release can pass a tree the hook would refuse.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
