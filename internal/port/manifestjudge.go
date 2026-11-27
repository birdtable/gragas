package port

import "github.com/birdtable/gragas/internal/model/plugin"

// ManifestJudge is the judgement of a plugin's manifest: nil for
// a manifest it has nothing against, and the refusal for one that holds what
// the manifest schema cannot. The engine's is plugins.Judge, a pure function.
// It is named here because both sides of the port hold it: main hands it to
// host's opener, which refuses to be made without one and asks it before
// anything of a bundle is read further, and the approval screen reaches the
// same one through the core, so no manifest is opened or shown unjudged and
// the two get one answer.
type ManifestJudge func(m plugin.Manifest) error
