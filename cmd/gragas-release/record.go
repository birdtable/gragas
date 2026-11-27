package main

import (
	"path/filepath"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/ordered"
)

// record is what the unpack step found, written beside the release folders
// so the release's own record says what was checked (supply.release.6). It is
// written whether the step passed or not, and never shipped: the draft's
// assets are the checksum file and its archives alone.
type record struct {
	Tag     string          `json:"tag"`
	Result  string          `json:"result"`
	Failure string          `json:"failure,omitempty"`
	Sums    string          `json:"checksums_sha256,omitempty"`
	Host    string          `json:"host,omitempty"`
	Network string          `json:"network,omitempty"`
	Started []string        `json:"started,omitempty"`
	Checked []recordArchive `json:"archives,omitempty"`
}

// recordArchive is one archive the step held: its digest, and the digest of
// the binary it unpacks to, which is the built binary's.
type recordArchive struct {
	Name   string `json:"name"`
	System string `json:"system"`
	SHA256 string `json:"sha256"`
	Binary string `json:"binary_sha256"`
}

// The results a record holds.
const (
	recordPass = "pass"
	recordFail = "fail"
)

// writeRecord writes r as JSON to path, whole or not at all, through the
// host's one writer (gate/shared.yaml, atomic-write). The bytes come from
// internal/ordered, the one JSON writer outside the root command's answer,
// so the encoder ban of cli.output.2 holds this file with no suppression
// (gragas#1750).
func writeRecord(path string, r record) error {
	tree := &hostfs.Tree{}
	root, err := tree.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	return hostfs.Writer{Tree: tree}.Replace(root, filepath.Base(path), []byte(ordered.JSON(r.doc(), 2)+"\n"))
}

// doc is r as the record's document: its fields in order under their JSON
// names, an empty one left out.
func (r record) doc() *ordered.Map {
	m := ordered.NewMap(0)
	m.Set("tag", r.Tag, 0)
	m.Set("result", r.Result, 0)
	for _, f := range []struct{ name, value string }{{"failure", r.Failure}, {"checksums_sha256", r.Sums}, {"host", r.Host}, {"network", r.Network}} {
		if f.value != "" {
			m.Set(f.name, f.value, 0)
		}
	}
	if len(r.Started) > 0 {
		started := make([]any, 0, len(r.Started))
		for _, s := range r.Started {
			started = append(started, s)
		}
		m.Set("started", started, 0)
	}
	if len(r.Checked) > 0 {
		archives := make([]any, 0, len(r.Checked))
		for _, a := range r.Checked {
			am := ordered.NewMap(0)
			am.Set("name", a.Name, 0)
			am.Set("system", a.System, 0)
			am.Set("sha256", a.SHA256, 0)
			am.Set("binary_sha256", a.Binary, 0)
			archives = append(archives, am)
		}
		m.Set("archives", archives, 0)
	}
	return m
}
