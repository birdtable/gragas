package coreapi

import (
	"context"
	"errors"
	"io/fs"

	"github.com/birdtable/gragas/internal/model"
	modelplugin "github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// grantsSince compares held, the grants of the bundle in hand, with the newest
// approval of the plugin called name. A first approval, and a
// prior one that kept no grants by id, are not compared: the screen shows every
// grant. A record Trust refuses is an error, never "a first approval".
func grantsSince(ctx context.Context, t port.Trust, name string, held []model.GrantHeld) (model.GrantsSince, error) {
	prior, err := t.Prior(ctx, name)
	if errors.Is(err, fs.ErrNotExist) {
		return model.GrantsSince{Changes: []model.GrantChange{}}, nil
	}
	if err != nil {
		return model.GrantsSince{}, err
	}
	out := model.GrantsSince{Prior: modelplugin.Shown(prior.Ref), Recorded: prior.Held != nil, Changes: []model.GrantChange{}}
	if !out.Recorded {
		return out, nil
	}
	out.Changes = changesBetween(prior.Held, held)
	return out, nil
}

// changesBetween walks two lists sorted by id together: an id only in now is
// added, only in before removed, and in both with another digest changed.
func changesBetween(before, now []model.GrantHeld) []model.GrantChange {
	out := []model.GrantChange{}
	i, j := 0, 0
	for i < len(before) || j < len(now) {
		switch {
		case j == len(now) || (i < len(before) && before[i].ID < now[j].ID):
			out = append(out, model.GrantChange{ID: modelplugin.Shown(before[i].ID), Change: model.GrantRemoved})
			i++
		case i == len(before) || now[j].ID < before[i].ID:
			out = append(out, model.GrantChange{ID: modelplugin.Shown(now[j].ID), Change: model.GrantAdded})
			j++
		default:
			if before[i].Digest != now[j].Digest {
				out = append(out, model.GrantChange{ID: modelplugin.Shown(now[j].ID), Change: model.GrantChanged})
			}
			i++
			j++
		}
	}
	return out
}
