---
type: reason
status: current
as-of: 2026-11-16
---

# Lint for unhashed inline scripts in Astro

The astro lint rule refuses inline scripts that the content security policy would block.

- **astro.lint.1** (Hard, active): Neither hole had a row in the stack list. Under the generated script policy an unhashed inline script fails loudly at run time, so the lint is a convenience as well as a net.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
