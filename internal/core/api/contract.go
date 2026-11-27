package coreapi

// The contract commands' compositions: validate checks one document against
// the contract's schemas, fields prints one entity's fields with their
// versions, and fixture writes the fixture rulebook's documents to a folder.
// The rules are core/contract's; these read and write through the ports.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/birdtable/gragas/internal/core/contract"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ErrFolderBusy is a folder whose root lock another run holds: the fixture
// ends busy and writes nothing.
var ErrFolderBusy = errors.New("another run holds the lock")

func kindNames() []string {
	var out []string
	for _, k := range contract.Kinds() {
		out = append(out, string(k))
	}
	return out
}

// ContractValidate reads the file at name, any file on the machine, and checks
// it as a document of kind, or of the kind its top-level fields name when kind
// is empty. A kind that is none, or a document of no one kind, is a usage that
// lists the kinds; a newer contract major is told with both versions.
func ContractValidate(p port.Ports, name, kind string) (model.ContractCheck, error) {
	out := model.ContractCheck{Schema: model.ContractCheckSchema, Findings: []model.Finding{}}
	dir, base := path.Split(name)
	root, err := p.Tree.Open(path.Clean(dirOrDot(dir)))
	if err != nil {
		return out, fmt.Errorf("the folder of %s could not be opened: %w", name, err)
	}
	doc, err := p.Tree.Read(root, base)
	if err != nil {
		return out, fmt.Errorf("%s could not be read: %w", name, err)
	}
	var k contract.Kind
	if kind != "" {
		var ok bool
		if k, ok = contract.ParseKind(kind); !ok {
			out.Usage = fmt.Sprintf("%q is not a kind of document", kind)
			out.Known = kindNames()
			return out, nil
		}
	} else {
		var ok bool
		if k, ok = contract.SniffKind(doc); !ok {
			out.Usage = "the kind of " + name + " cannot be told from its fields; name it with --as"
			out.Known = kindNames()
			return out, nil
		}
	}
	out.Kind = string(k)
	v, err := contract.Validate(k, doc, contract.Schemas{Check: p.Schema, Files: p.Assets, Cap: contract.MaxDocument})
	if nr, ok := errors.AsType[*contract.NotReady](err); ok {
		out.NotReady = &model.ContractNotReady{Found: nr.Found.String(), Reads: nr.Reads.String()}
		out.Contract = nr.Found.String()
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if v.Contract != (contract.Version{}) {
		out.Contract = v.Contract.String()
	}
	for _, f := range v.Findings {
		if f.File == "" {
			f.File = name
		}
		out.Findings = append(out.Findings, f)
	}
	return out, nil
}

func dirOrDot(d string) string {
	if d == "" {
		return "."
	}
	return d
}

// ContractFields is the fields of the entity the contract names, with the
// version each came in. An entity the contract lacks is a usage that lists
// the ones it has.
func ContractFields(p port.Ports, entity string) (model.ContractFields, error) {
	out := model.ContractFields{Schema: model.ContractFieldsSchema, Entity: entity, Fields: []model.ContractField{}}
	f, err := contract.FieldsOf(entity, p.Assets)
	if unknown, ok := errors.AsType[*contract.UnknownEntity](err); ok {
		out.Usage = unknown.Error()
		out.Known = unknown.Known
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Source = f.Schema
	out.Contract = f.Contract.String()
	for _, r := range f.Rows {
		out.Fields = append(out.Fields, model.ContractField{Name: r.Name, Type: r.Type, Since: r.Since, Default: r.Default})
	}
	return out, nil
}

// ContractFixture writes the fixture's documents into the folder out, which
// must exist, under the root lock and each by the Writer's replace, and
// nothing outside out. It is a report write: no plan, no journal and no
// history line. Without apply it lists what it would write and writes nothing.
func ContractFixture(ctx context.Context, p port.Ports, out string, apply bool) (model.ContractFixture, error) {
	res := model.ContractFixture{Schema: model.ContractFixtureSchema, Out: out, Files: []string{}}
	files, err := contract.Fixture(p.Assets)
	if err != nil {
		return res, err
	}
	for _, f := range files {
		res.Files = append(res.Files, f.Path)
	}
	if !apply {
		return res, nil
	}
	root, err := p.Tree.Open(out)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return res, fmt.Errorf("the folder %s does not exist; make it first: %w", out, err)
		}
		return res, fmt.Errorf("%s could not be opened: %w", out, err)
	}
	hold, err := p.Lock.Take(ctx, root, model.RootLock)
	if err != nil {
		return res, fmt.Errorf("the folder %s is busy (%w): %w", out, ErrFolderBusy, err)
	}
	var werr error
	for _, f := range files {
		if werr = p.Writer.Replace(root, path.Clean(f.Path), f.Data); werr != nil {
			werr = fmt.Errorf("%s could not be written: %w", f.Path, werr)
			break
		}
	}
	if rerr := p.Lock.Release(hold); rerr != nil && werr == nil {
		werr = fmt.Errorf("the lock on %s could not be put back: %w", out, rerr)
	}
	if werr != nil {
		return res, werr
	}
	res.Wrote = true
	return res, nil
}
