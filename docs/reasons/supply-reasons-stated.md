---
type: reason
status: current
as-of: 2026-11-16
---

# Reasons for supply-chain rules that carry them on this page

These supply-chain rules carry their reason in this one page. Each keeps its own status.

- **supply.age.4** (Hard, active): A release that loses the provenance attestation its predecessors had is the signature of a stolen publishing token.
- **supply.bumps.8** (Lens, active): One set of design-system releases reached its main web application in 2 hours 45 minutes, 5 hours 30 minutes, and once not in 5 days, because nothing linked a publish to its consumer.
- **supply.install.10** (Hard, active): A check that fetches its own tool at run time can be replaced by whatever has that name, and a changed version gives a changed verdict.
- **supply.install.6** (Hard, active): An install script runs arbitrary code on every machine that installs, which is the usual route of a package takeover.
- **supply.provenance.2** (Hard, active): A copy with no origin cannot be updated or checked against its source.
- **supply.release.10** (Lens, active): One project found its broken release config only at release, because the dry run was not a member of the gate; a release from a working checkout carries whatever that checkout holds.
- **supply.release.11** (Lens, active): Hosted release workflows in one group of projects installed per job and had no success on record in seven repositories; a release that builds once cannot say its bytes are reproducible.
- **supply.release.2** (Hard, active): A break the author did not declare turns every consumer's next bump into a surprise. The tag step is the last moment the check can run, because the tool refuses once the tag exists. A real break was found in the shared module.
- **supply.release.7** (Hard, active): A compile fault is invisible until a later tier if the push does not vet at all. (An earlier version of this rule asked for a vet per shipped GOOS, because one program built only on Linux failed in six packages on Linux-only calls; a product now holds no such calls.)
- **supply.secrets.2** (Hard, active): A value in configuration is copied into logs, images and every place the configuration goes; a reference is resolved only where it is used.
- **supply.workflows.2** (Hard, active): Injected faults in workflow files were found by the linter and the audit in a planted trial: three of three. One repository had two high audit findings for a token with no permission input.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
