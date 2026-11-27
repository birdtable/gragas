package main

import (
	"context"
	"strings"
)

// releaseBaseRef is the clone's copy of the release branch, named in
// full for the reason baseRef is.
const releaseBaseRef = "refs/remotes/origin/release-1"

// basesFor are the revisions a head's change is held to (gragas#1866), each
// one a base a row it holds may not be lost against:
//
//   - the main the head carries: its merge base with origin/main, so a head
//     bound for main that merged the release in is still held to main's rows,
//     and a release head that never took main is held to the cut, not to a
//     main that has gone on since;
//   - the release's, when the head descends from a commit of
//     origin/main..origin/release-1 (its merge base with the release is not
//     already in main), the rule the pre-push merge check takes its base by
//     (gragas#1733): the release's own rows may not be lost either.
//
// A clone with no origin/release-1, or a head with nothing in common with it,
// is held to main as before; so is one with no merge base with main.
func basesFor(ctx context.Context, environ []string, dir, head string) []string {
	bases := []string{mainBaseOf(ctx, environ, dir, head)}
	if _, err := output(ctx, environ, dir, nil, "git", "rev-parse", "--verify", "--quiet", releaseBaseRef+"^{commit}"); err != nil {
		return bases
	}
	if base := mergeBaseOf(ctx, environ, dir, head, releaseBaseRef); base != "" && !isAncestor(ctx, environ, dir, base, baseRef) {
		bases = append(bases, releaseBaseRef)
	}
	return bases
}

// mainBaseOf is the merge base of head and origin/main, or origin/main itself
// when they share none.
func mainBaseOf(ctx context.Context, environ []string, dir, head string) string {
	if base := mergeBaseOf(ctx, environ, dir, head, baseRef); base != "" {
		return base
	}
	return baseRef
}

// mergeBaseOf is the merge base of a and b, empty when git names none.
func mergeBaseOf(ctx context.Context, environ []string, dir, a, b string) string {
	out, err := output(ctx, environ, dir, nil, "git", "merge-base", a, b)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// isAncestor is whether git says a is an ancestor of b (exit 0). A run that
// fails is no too, so basesFor then holds the head to the release as well: a
// check that cannot tell holds more, never less.
func isAncestor(ctx context.Context, environ []string, dir, a, b string) bool {
	_, err := output(ctx, environ, dir, nil, "git", "merge-base", "--is-ancestor", a, b)
	return err == nil
}
