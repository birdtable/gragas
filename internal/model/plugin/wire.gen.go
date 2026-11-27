// Written by gragas-generate from contract/plugin/v1/wire.schema.json; edits are lost, so edit the source and run task gen.

package plugin

import "github.com/birdtable/gragas/internal/canon"

// Target is one of the strings source, config, migration, container, workflow, iac, api-spec, prose, commit, telemetry-registry, dependency, package lists.
type Target string

const (
	// TargetSource is "source".
	TargetSource Target = "source"
	// TargetConfig is "config".
	TargetConfig Target = "config"
	// TargetMigration is "migration".
	TargetMigration Target = "migration"
	// TargetContainer is "container".
	TargetContainer Target = "container"
	// TargetWorkflow is "workflow".
	TargetWorkflow Target = "workflow"
	// TargetIac is "iac".
	TargetIac Target = "iac"
	// TargetAPISpec is "api-spec".
	TargetAPISpec Target = "api-spec"
	// TargetProse is "prose".
	TargetProse Target = "prose"
	// TargetCommit is "commit".
	TargetCommit Target = "commit"
	// TargetTelemetryRegistry is "telemetry-registry".
	TargetTelemetryRegistry Target = "telemetry-registry"
	// TargetDependency is "dependency".
	TargetDependency Target = "dependency"
	// TargetPackage is "package".
	TargetPackage Target = "package"
)

// Valid says whether Target is one of the values.
func (e Target) Valid() bool {
	switch e {
	case TargetSource, TargetConfig, TargetMigration, TargetContainer, TargetWorkflow, TargetIac, TargetAPISpec, TargetProse, TargetCommit, TargetTelemetryRegistry, TargetDependency, TargetPackage:
		return true
	}
	return false
}

// Severity is one of the strings error, warning, info lists.
type Severity string

const (
	// SeverityError is "error".
	SeverityError Severity = "error"
	// SeverityWarning is "warning".
	SeverityWarning Severity = "warning"
	// SeverityInfo is "info".
	SeverityInfo Severity = "info"
)

// Valid says whether Severity is one of the values.
func (e Severity) Valid() bool {
	switch e {
	case SeverityError, SeverityWarning, SeverityInfo:
		return true
	}
	return false
}

// Level is one of the strings isolated, sandboxed lists.
type Level string

const (
	// LevelIsolated is "isolated".
	LevelIsolated Level = "isolated"
	// LevelSandboxed is "sandboxed".
	LevelSandboxed Level = "sandboxed"
)

// Valid says whether Level is one of the values.
func (e Level) Valid() bool {
	switch e {
	case LevelIsolated, LevelSandboxed:
		return true
	}
	return false
}

// Bytes is a message or part of one of plugin contract 1.
type Bytes struct {
	Text   *string `json:"text"`
	Base64 *string `json:"base64"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Bytes) ToCanon() canon.Value {
	o := canon.Object{}
	if x.Text != nil {
		o["text"] = canon.String((*x.Text))
	}
	if x.Base64 != nil {
		o["base64"] = canon.String((*x.Base64))
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Bytes) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Bytes) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "text", "base64"); err != nil {
		return err
	}
	if err := exactlyOne(o, p, "text", "base64"); err != nil {
		return err
	}
	{
		m, ok := o["text"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "text"), 0, -1, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Text = &s
		}
	}
	{
		m, ok := o["base64"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "base64"), 0, -1, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Base64 = &s
		}
	}
	return nil
}

// File is a message or part of one of plugin contract 1.
type File struct {
	Path string `json:"path"`
	Data Bytes  `json:"data"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x File) ToCanon() canon.Value {
	o := canon.Object{}
	o["path"] = canon.String(x.Path)
	o["data"] = x.Data.ToCanon()
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *File) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *File) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "path", "data"); err != nil {
		return err
	}
	{
		m, ok := o["path"]
		if !ok {
			return fault(p, "has no member path")
		}
		s1, err := text(m, pointer(p, "path"), 1, 4096, nil)
		if err != nil {
			return err
		}
		x.Path = s1
	}
	{
		m, ok := o["data"]
		if !ok {
			return fault(p, "has no member data")
		}
		if err := x.Data.read(m, pointer(p, "data")); err != nil {
			return err
		}
	}
	return nil
}

// Config is a message or part of one of plugin contract 1.
type Config struct {
	Path string `json:"path"`
	Data *Bytes `json:"data"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Config) ToCanon() canon.Value {
	o := canon.Object{}
	o["path"] = canon.String(x.Path)
	if x.Data != nil {
		o["data"] = (*x.Data).ToCanon()
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Config) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Config) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["path"]
		if !ok {
			return fault(p, "has no member path")
		}
		s1, err := text(m, pointer(p, "path"), 1, 4096, nil)
		if err != nil {
			return err
		}
		x.Path = s1
	}
	{
		m, ok := o["data"]
		if ok {
			var s Bytes
			if err := s.read(m, pointer(p, "data")); err != nil {
				return err
			}
			x.Data = &s
		}
	}
	return nil
}

// Run is a message or part of one of plugin contract 1.
type Run struct {
	Program string   `json:"program"`
	Argv    []string `json:"argv"`
	Stdin   *Bytes   `json:"stdin"`
	Place   []File   `json:"place"`
	Collect []string `json:"collect"`
	Level   *Level   `json:"level"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Run) ToCanon() canon.Value {
	o := canon.Object{}
	o["kind"] = canon.String("run")
	o["program"] = canon.String(x.Program)
	o["argv"] = listOf(x.Argv, func(x0 string) canon.Value { return canon.String(x0) })
	if x.Stdin != nil {
		o["stdin"] = (*x.Stdin).ToCanon()
	}
	if x.Place != nil {
		o["place"] = listOf(x.Place, func(x0 File) canon.Value { return x0.ToCanon() })
	}
	if x.Collect != nil {
		o["collect"] = listOf(x.Collect, func(x0 string) canon.Value { return canon.String(x0) })
	}
	if x.Level != nil {
		o["level"] = canon.String((*x.Level))
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Run) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Run) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "kind", "program", "argv", "stdin", "place", "collect", "level"); err != nil {
		return err
	}
	{
		m, ok := o["kind"]
		if !ok {
			return fault(p, "has no member kind")
		}
		if m != canon.String("run") {
			return fault(pointer(p, "kind"), "is not run")
		}
	}
	{
		m, ok := o["program"]
		if !ok {
			return fault(p, "has no member program")
		}
		s1, err := text(m, pointer(p, "program"), 1, 64, nil)
		if err != nil {
			return err
		}
		x.Program = s1
	}
	{
		m, ok := o["argv"]
		if !ok {
			return fault(p, "has no member argv")
		}
		a1, err := array(m, pointer(p, "argv"), 0, 256)
		if err != nil {
			return err
		}
		x.Argv = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "argv"), i1), 0, 4096, nil)
			if err != nil {
				return err
			}
			x.Argv[i1] = s2
		}
	}
	{
		m, ok := o["stdin"]
		if ok {
			var s Bytes
			if err := s.read(m, pointer(p, "stdin")); err != nil {
				return err
			}
			x.Stdin = &s
		}
	}
	{
		m, ok := o["place"]
		if ok {
			a1, err := array(m, pointer(p, "place"), 0, 16)
			if err != nil {
				return err
			}
			x.Place = make([]File, len(a1))
			for i1, e1 := range a1 {
				if err := x.Place[i1].read(e1, index(pointer(p, "place"), i1)); err != nil {
					return err
				}
			}
		}
	}
	{
		m, ok := o["collect"]
		if ok {
			a1, err := array(m, pointer(p, "collect"), 0, 16)
			if err != nil {
				return err
			}
			x.Collect = make([]string, len(a1))
			for i1, e1 := range a1 {
				s2, err := text(e1, index(pointer(p, "collect"), i1), 1, 4096, nil)
				if err != nil {
					return err
				}
				x.Collect[i1] = s2
			}
		}
	}
	{
		m, ok := o["level"]
		if ok {
			var s Level
			s1, err := text(m, pointer(p, "level"), 0, -1, nil)
			if err != nil {
				return err
			}
			if !Level(s1).Valid() {
				return fault(pointer(p, "level"), "is not one of isolated, sandboxed")
			}
			s = Level(s1)
			x.Level = &s
		}
	}
	return nil
}

// Read is a message or part of one of plugin contract 1.
type Read struct {
	Glob string `json:"glob"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Read) ToCanon() canon.Value {
	o := canon.Object{}
	o["kind"] = canon.String("read")
	o["glob"] = canon.String(x.Glob)
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Read) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Read) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "kind", "glob"); err != nil {
		return err
	}
	{
		m, ok := o["kind"]
		if !ok {
			return fault(p, "has no member kind")
		}
		if m != canon.String("read") {
			return fault(pointer(p, "kind"), "is not read")
		}
	}
	{
		m, ok := o["glob"]
		if !ok {
			return fault(p, "has no member glob")
		}
		s1, err := text(m, pointer(p, "glob"), 1, 1024, nil)
		if err != nil {
			return err
		}
		x.Glob = s1
	}
	return nil
}

// Kind is one of the strings run, read lists.
type Kind string

const (
	// KindRun is "run".
	KindRun Kind = "run"
	// KindRead is "read".
	KindRead Kind = "read"
)

// Valid says whether Kind is one of the values.
func (e Kind) Valid() bool {
	switch e {
	case KindRun, KindRead:
		return true
	}
	return false
}

// Result is a message or part of one of plugin contract 1.
type Result struct {
	Round  int    `json:"round"`
	Step   int    `json:"step"`
	Kind   Kind   `json:"kind"`
	Exit   *int   `json:"exit"`
	Stdout *Bytes `json:"stdout"`
	Stderr *Bytes `json:"stderr"`
	Cut    *bool  `json:"cut"`
	Files  []File `json:"files"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Result) ToCanon() canon.Value {
	o := canon.Object{}
	o["round"] = canon.Int(x.Round)
	o["step"] = canon.Int(x.Step)
	o["kind"] = canon.String(x.Kind)
	if x.Exit != nil {
		o["exit"] = canon.Int((*x.Exit))
	}
	if x.Stdout != nil {
		o["stdout"] = (*x.Stdout).ToCanon()
	}
	if x.Stderr != nil {
		o["stderr"] = (*x.Stderr).ToCanon()
	}
	if x.Cut != nil {
		o["cut"] = canon.Bool((*x.Cut))
	}
	o["files"] = listOf(x.Files, func(x0 File) canon.Value { return x0.ToCanon() })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Result) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Result) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := needs(o, p, "kind", canon.String("run"), "exit", "stdout", "stderr", "cut"); err != nil {
		return err
	}
	{
		m, ok := o["round"]
		if !ok {
			return fault(p, "has no member round")
		}
		n1, err := whole(m, pointer(p, "round"), 0, 2)
		if err != nil {
			return err
		}
		x.Round = n1
	}
	{
		m, ok := o["step"]
		if !ok {
			return fault(p, "has no member step")
		}
		n1, err := whole(m, pointer(p, "step"), 0, 15)
		if err != nil {
			return err
		}
		x.Step = n1
	}
	{
		m, ok := o["kind"]
		if !ok {
			return fault(p, "has no member kind")
		}
		s1, err := text(m, pointer(p, "kind"), 0, -1, nil)
		if err != nil {
			return err
		}
		if !Kind(s1).Valid() {
			return fault(pointer(p, "kind"), "is not one of run, read")
		}
		x.Kind = Kind(s1)
	}
	{
		m, ok := o["exit"]
		if ok {
			var s int
			n1, err := whole(m, pointer(p, "exit"), -9007199254740991, 9007199254740991)
			if err != nil {
				return err
			}
			s = n1
			x.Exit = &s
		}
	}
	{
		m, ok := o["stdout"]
		if ok {
			var s Bytes
			if err := s.read(m, pointer(p, "stdout")); err != nil {
				return err
			}
			x.Stdout = &s
		}
	}
	{
		m, ok := o["stderr"]
		if ok {
			var s Bytes
			if err := s.read(m, pointer(p, "stderr")); err != nil {
				return err
			}
			x.Stderr = &s
		}
	}
	{
		m, ok := o["cut"]
		if ok {
			var s bool
			t1, err := truth(m, pointer(p, "cut"))
			if err != nil {
				return err
			}
			s = t1
			x.Cut = &s
		}
	}
	{
		m, ok := o["files"]
		if !ok {
			return fault(p, "has no member files")
		}
		a1, err := array(m, pointer(p, "files"), 0, 256)
		if err != nil {
			return err
		}
		x.Files = make([]File, len(a1))
		for i1, e1 := range a1 {
			if err := x.Files[i1].read(e1, index(pointer(p, "files"), i1)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Gathered is a message or part of one of plugin contract 1.
type Gathered struct {
	ToolVersion string   `json:"tool_version"`
	Scratch     *string  `json:"scratch"`
	Config      []Config `json:"config"`
	Have        []Result `json:"have"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Gathered) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool_version"] = canon.String(x.ToolVersion)
	if x.Scratch != nil {
		o["scratch"] = canon.String((*x.Scratch))
	}
	o["config"] = listOf(x.Config, func(x0 Config) canon.Value { return x0.ToCanon() })
	o["have"] = listOf(x.Have, func(x0 Result) canon.Value { return x0.ToCanon() })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Gathered) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Gathered) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["tool_version"]
		if !ok {
			return fault(p, "has no member tool_version")
		}
		s1, err := text(m, pointer(p, "tool_version"), 0, 256, pattern0)
		if err != nil {
			return err
		}
		x.ToolVersion = s1
	}
	{
		m, ok := o["scratch"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "scratch"), 0, 4096, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Scratch = &s
		}
	}
	{
		m, ok := o["config"]
		if !ok {
			return fault(p, "has no member config")
		}
		a1, err := array(m, pointer(p, "config"), 0, 64)
		if err != nil {
			return err
		}
		x.Config = make([]Config, len(a1))
		for i1, e1 := range a1 {
			if err := x.Config[i1].read(e1, index(pointer(p, "config"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["have"]
		if !ok {
			return fault(p, "has no member have")
		}
		a1, err := array(m, pointer(p, "have"), 0, 48)
		if err != nil {
			return err
		}
		x.Have = make([]Result, len(a1))
		for i1, e1 := range a1 {
			if err := x.Have[i1].read(e1, index(pointer(p, "have"), i1)); err != nil {
				return err
			}
		}
	}
	return nil
}

// For is one of the strings config, run lists.
type For string

const (
	// ForConfig is "config".
	ForConfig For = "config"
	// ForRun is "run".
	ForRun For = "run"
)

// Valid says whether For is one of the values.
func (e For) Valid() bool {
	switch e {
	case ForConfig, ForRun:
		return true
	}
	return false
}

// PlanRequest is a message or part of one of plugin contract 1.
type PlanRequest struct {
	ToolVersion string   `json:"tool_version"`
	Scratch     *string  `json:"scratch"`
	Config      []Config `json:"config"`
	Have        []Result `json:"have"`
	For         For      `json:"for"`
	Target      Target   `json:"target"`
	Round       int      `json:"round"`
	Fixtures    []string `json:"fixtures"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x PlanRequest) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool_version"] = canon.String(x.ToolVersion)
	if x.Scratch != nil {
		o["scratch"] = canon.String((*x.Scratch))
	}
	o["config"] = listOf(x.Config, func(x0 Config) canon.Value { return x0.ToCanon() })
	o["have"] = listOf(x.Have, func(x0 Result) canon.Value { return x0.ToCanon() })
	o["for"] = canon.String(x.For)
	o["target"] = canon.String(x.Target)
	o["round"] = canon.Int(x.Round)
	if x.Fixtures != nil {
		o["fixtures"] = listOf(x.Fixtures, func(x0 string) canon.Value { return canon.String(x0) })
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *PlanRequest) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *PlanRequest) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["tool_version"]
		if !ok {
			return fault(p, "has no member tool_version")
		}
		s1, err := text(m, pointer(p, "tool_version"), 0, 256, pattern0)
		if err != nil {
			return err
		}
		x.ToolVersion = s1
	}
	{
		m, ok := o["scratch"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "scratch"), 0, 4096, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Scratch = &s
		}
	}
	{
		m, ok := o["config"]
		if !ok {
			return fault(p, "has no member config")
		}
		a1, err := array(m, pointer(p, "config"), 0, 64)
		if err != nil {
			return err
		}
		x.Config = make([]Config, len(a1))
		for i1, e1 := range a1 {
			if err := x.Config[i1].read(e1, index(pointer(p, "config"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["have"]
		if !ok {
			return fault(p, "has no member have")
		}
		a1, err := array(m, pointer(p, "have"), 0, 48)
		if err != nil {
			return err
		}
		x.Have = make([]Result, len(a1))
		for i1, e1 := range a1 {
			if err := x.Have[i1].read(e1, index(pointer(p, "have"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["for"]
		if !ok {
			return fault(p, "has no member for")
		}
		s1, err := text(m, pointer(p, "for"), 0, -1, nil)
		if err != nil {
			return err
		}
		if !For(s1).Valid() {
			return fault(pointer(p, "for"), "is not one of config, run")
		}
		x.For = For(s1)
	}
	{
		m, ok := o["target"]
		if !ok {
			return fault(p, "has no member target")
		}
		s1, err := text(m, pointer(p, "target"), 0, -1, nil)
		if err != nil {
			return err
		}
		if !Target(s1).Valid() {
			return fault(pointer(p, "target"), "is not one of source, config, migration, container, workflow, iac, api-spec, prose, commit, telemetry-registry, dependency, package")
		}
		x.Target = Target(s1)
	}
	{
		m, ok := o["round"]
		if !ok {
			return fault(p, "has no member round")
		}
		n1, err := whole(m, pointer(p, "round"), 0, 2)
		if err != nil {
			return err
		}
		x.Round = n1
	}
	{
		m, ok := o["fixtures"]
		if ok {
			a1, err := array(m, pointer(p, "fixtures"), 0, 1024)
			if err != nil {
				return err
			}
			x.Fixtures = make([]string, len(a1))
			for i1, e1 := range a1 {
				s2, err := text(e1, index(pointer(p, "fixtures"), i1), 1, 4096, nil)
				if err != nil {
					return err
				}
				x.Fixtures[i1] = s2
			}
		}
	}
	return nil
}

// Step is one of Run, Read, by the alternative it holds; exactly one of its members is set.
type Step struct {
	// Run is set when the member is a Run.
	Run *Run
	// Read is set when the member is a Read.
	Read *Read
}

// ToCanon is the alternative that is set; with none set it is null, which no
// schema accepts.
func (x Step) ToCanon() canon.Value {
	switch {
	case x.Run != nil:
		return x.Run.ToCanon()
	case x.Read != nil:
		return x.Read.ToCanon()
	}
	return canon.Null{}
}

// FromCanon reads v into the alternative it is.
func (x *Step) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Step) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	m, ok := o["kind"]
	if !ok {
		return fault(p, "has no member kind")
	}
	switch m {
	case canon.String("run"):
		x.Run = new(Run)
		return x.Run.read(v, p)
	case canon.String("read"):
		x.Read = new(Read)
		return x.Read.read(v, p)
	}
	return fault(pointer(p, "kind"), "is not one of run, read")
}

// PlanResponse is a message or part of one of plugin contract 1.
type PlanResponse struct {
	Steps []Step `json:"steps"`
	Final bool   `json:"final"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x PlanResponse) ToCanon() canon.Value {
	o := canon.Object{}
	o["steps"] = listOf(x.Steps, func(x0 Step) canon.Value { return x0.ToCanon() })
	o["final"] = canon.Bool(x.Final)
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *PlanResponse) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *PlanResponse) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "steps", "final"); err != nil {
		return err
	}
	{
		m, ok := o["steps"]
		if !ok {
			return fault(p, "has no member steps")
		}
		a1, err := array(m, pointer(p, "steps"), 0, 16)
		if err != nil {
			return err
		}
		x.Steps = make([]Step, len(a1))
		for i1, e1 := range a1 {
			if err := x.Steps[i1].read(e1, index(pointer(p, "steps"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["final"]
		if !ok {
			return fault(p, "has no member final")
		}
		t1, err := truth(m, pointer(p, "final"))
		if err != nil {
			return err
		}
		x.Final = t1
	}
	return nil
}

// EntriesRequest is a message or part of one of plugin contract 1.
type EntriesRequest struct {
	ToolVersion string   `json:"tool_version"`
	Scratch     *string  `json:"scratch"`
	Config      []Config `json:"config"`
	Have        []Result `json:"have"`
	Target      Target   `json:"target"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x EntriesRequest) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool_version"] = canon.String(x.ToolVersion)
	if x.Scratch != nil {
		o["scratch"] = canon.String((*x.Scratch))
	}
	o["config"] = listOf(x.Config, func(x0 Config) canon.Value { return x0.ToCanon() })
	o["have"] = listOf(x.Have, func(x0 Result) canon.Value { return x0.ToCanon() })
	o["target"] = canon.String(x.Target)
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *EntriesRequest) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *EntriesRequest) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["tool_version"]
		if !ok {
			return fault(p, "has no member tool_version")
		}
		s1, err := text(m, pointer(p, "tool_version"), 0, 256, pattern0)
		if err != nil {
			return err
		}
		x.ToolVersion = s1
	}
	{
		m, ok := o["scratch"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "scratch"), 0, 4096, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Scratch = &s
		}
	}
	{
		m, ok := o["config"]
		if !ok {
			return fault(p, "has no member config")
		}
		a1, err := array(m, pointer(p, "config"), 0, 64)
		if err != nil {
			return err
		}
		x.Config = make([]Config, len(a1))
		for i1, e1 := range a1 {
			if err := x.Config[i1].read(e1, index(pointer(p, "config"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["have"]
		if !ok {
			return fault(p, "has no member have")
		}
		a1, err := array(m, pointer(p, "have"), 0, 48)
		if err != nil {
			return err
		}
		x.Have = make([]Result, len(a1))
		for i1, e1 := range a1 {
			if err := x.Have[i1].read(e1, index(pointer(p, "have"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["target"]
		if !ok {
			return fault(p, "has no member target")
		}
		s1, err := text(m, pointer(p, "target"), 0, -1, nil)
		if err != nil {
			return err
		}
		if !Target(s1).Valid() {
			return fault(pointer(p, "target"), "is not one of source, config, migration, container, workflow, iac, api-spec, prose, commit, telemetry-registry, dependency, package")
		}
		x.Target = Target(s1)
	}
	return nil
}

// Entry is a message or part of one of plugin contract 1.
type Entry struct {
	Key      string      `json:"key"`
	On       bool        `json:"on"`
	Severity *Severity   `json:"severity"`
	Value    canon.Value `json:"value"`
	Options  canon.Value `json:"options"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Entry) ToCanon() canon.Value {
	o := canon.Object{}
	o["key"] = canon.String(x.Key)
	o["on"] = canon.Bool(x.On)
	if x.Severity != nil {
		o["severity"] = canon.String((*x.Severity))
	}
	if x.Value != nil {
		o["value"] = x.Value
	}
	if x.Options != nil {
		o["options"] = x.Options
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Entry) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Entry) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "key", "on", "severity", "value", "options"); err != nil {
		return err
	}
	{
		m, ok := o["key"]
		if !ok {
			return fault(p, "has no member key")
		}
		s1, err := text(m, pointer(p, "key"), 1, 512, nil)
		if err != nil {
			return err
		}
		x.Key = s1
	}
	{
		m, ok := o["on"]
		if !ok {
			return fault(p, "has no member on")
		}
		t1, err := truth(m, pointer(p, "on"))
		if err != nil {
			return err
		}
		x.On = t1
	}
	{
		m, ok := o["severity"]
		if ok {
			var s Severity
			s1, err := text(m, pointer(p, "severity"), 0, -1, nil)
			if err != nil {
				return err
			}
			if !Severity(s1).Valid() {
				return fault(pointer(p, "severity"), "is not one of error, warning, info")
			}
			s = Severity(s1)
			x.Severity = &s
		}
	}
	{
		m, ok := o["value"]
		if ok {
			x.Value = m
		}
	}
	{
		m, ok := o["options"]
		if ok {
			x.Options = m
		}
	}
	return nil
}

// EntriesResponse is a message or part of one of plugin contract 1.
type EntriesResponse struct {
	Entries []Entry `json:"entries"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x EntriesResponse) ToCanon() canon.Value {
	o := canon.Object{}
	o["entries"] = listOf(x.Entries, func(x0 Entry) canon.Value { return x0.ToCanon() })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *EntriesResponse) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *EntriesResponse) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "entries"); err != nil {
		return err
	}
	{
		m, ok := o["entries"]
		if !ok {
			return fault(p, "has no member entries")
		}
		a1, err := array(m, pointer(p, "entries"), 0, 100000)
		if err != nil {
			return err
		}
		x.Entries = make([]Entry, len(a1))
		for i1, e1 := range a1 {
			if err := x.Entries[i1].read(e1, index(pointer(p, "entries"), i1)); err != nil {
				return err
			}
		}
	}
	return nil
}

// FindingsRequest is a message or part of one of plugin contract 1.
type FindingsRequest struct {
	ToolVersion string   `json:"tool_version"`
	Scratch     *string  `json:"scratch"`
	Config      []Config `json:"config"`
	Have        []Result `json:"have"`
	Fixtures    []string `json:"fixtures"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x FindingsRequest) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool_version"] = canon.String(x.ToolVersion)
	if x.Scratch != nil {
		o["scratch"] = canon.String((*x.Scratch))
	}
	o["config"] = listOf(x.Config, func(x0 Config) canon.Value { return x0.ToCanon() })
	o["have"] = listOf(x.Have, func(x0 Result) canon.Value { return x0.ToCanon() })
	o["fixtures"] = listOf(x.Fixtures, func(x0 string) canon.Value { return canon.String(x0) })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *FindingsRequest) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *FindingsRequest) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["tool_version"]
		if !ok {
			return fault(p, "has no member tool_version")
		}
		s1, err := text(m, pointer(p, "tool_version"), 0, 256, pattern0)
		if err != nil {
			return err
		}
		x.ToolVersion = s1
	}
	{
		m, ok := o["scratch"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "scratch"), 0, 4096, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Scratch = &s
		}
	}
	{
		m, ok := o["config"]
		if !ok {
			return fault(p, "has no member config")
		}
		a1, err := array(m, pointer(p, "config"), 0, 64)
		if err != nil {
			return err
		}
		x.Config = make([]Config, len(a1))
		for i1, e1 := range a1 {
			if err := x.Config[i1].read(e1, index(pointer(p, "config"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["have"]
		if !ok {
			return fault(p, "has no member have")
		}
		a1, err := array(m, pointer(p, "have"), 0, 48)
		if err != nil {
			return err
		}
		x.Have = make([]Result, len(a1))
		for i1, e1 := range a1 {
			if err := x.Have[i1].read(e1, index(pointer(p, "have"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["fixtures"]
		if !ok {
			return fault(p, "has no member fixtures")
		}
		a1, err := array(m, pointer(p, "fixtures"), 0, 1024)
		if err != nil {
			return err
		}
		x.Fixtures = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "fixtures"), i1), 1, 4096, nil)
			if err != nil {
				return err
			}
			x.Fixtures[i1] = s2
		}
	}
	return nil
}

// Finding is a message or part of one of plugin contract 1.
type Finding struct {
	Key      string    `json:"key"`
	Message  string    `json:"message"`
	Path     *string   `json:"path"`
	Line     *int      `json:"line"`
	Column   *int      `json:"column"`
	Severity *Severity `json:"severity"`
	URL      *string   `json:"url"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Finding) ToCanon() canon.Value {
	o := canon.Object{}
	o["key"] = canon.String(x.Key)
	o["message"] = canon.String(x.Message)
	if x.Path != nil {
		o["path"] = canon.String((*x.Path))
	}
	if x.Line != nil {
		o["line"] = canon.Int((*x.Line))
	}
	if x.Column != nil {
		o["column"] = canon.Int((*x.Column))
	}
	if x.Severity != nil {
		o["severity"] = canon.String((*x.Severity))
	}
	if x.URL != nil {
		o["url"] = canon.String((*x.URL))
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Finding) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Finding) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "key", "message", "path", "line", "column", "severity", "url"); err != nil {
		return err
	}
	{
		m, ok := o["key"]
		if !ok {
			return fault(p, "has no member key")
		}
		s1, err := text(m, pointer(p, "key"), 1, 512, nil)
		if err != nil {
			return err
		}
		x.Key = s1
	}
	{
		m, ok := o["message"]
		if !ok {
			return fault(p, "has no member message")
		}
		s1, err := text(m, pointer(p, "message"), 0, 8192, nil)
		if err != nil {
			return err
		}
		x.Message = s1
	}
	{
		m, ok := o["path"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "path"), 1, 4096, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Path = &s
		}
	}
	{
		m, ok := o["line"]
		if ok {
			var s int
			n1, err := whole(m, pointer(p, "line"), 1, 4294967295)
			if err != nil {
				return err
			}
			s = n1
			x.Line = &s
		}
	}
	{
		m, ok := o["column"]
		if ok {
			var s int
			n1, err := whole(m, pointer(p, "column"), 1, 4294967295)
			if err != nil {
				return err
			}
			s = n1
			x.Column = &s
		}
	}
	{
		m, ok := o["severity"]
		if ok {
			var s Severity
			s1, err := text(m, pointer(p, "severity"), 0, -1, nil)
			if err != nil {
				return err
			}
			if !Severity(s1).Valid() {
				return fault(pointer(p, "severity"), "is not one of error, warning, info")
			}
			s = Severity(s1)
			x.Severity = &s
		}
	}
	{
		m, ok := o["url"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "url"), 0, 2048, nil)
			if err != nil {
				return err
			}
			s = s1
			x.URL = &s
		}
	}
	return nil
}

// FindingsResponse is a message or part of one of plugin contract 1.
type FindingsResponse struct {
	Ran      bool      `json:"ran"`
	Why      *string   `json:"why"`
	Findings []Finding `json:"findings"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x FindingsResponse) ToCanon() canon.Value {
	o := canon.Object{}
	o["ran"] = canon.Bool(x.Ran)
	if x.Why != nil {
		o["why"] = canon.String((*x.Why))
	}
	o["findings"] = listOf(x.Findings, func(x0 Finding) canon.Value { return x0.ToCanon() })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *FindingsResponse) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *FindingsResponse) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "ran", "why", "findings"); err != nil {
		return err
	}
	{
		m, ok := o["ran"]
		if !ok {
			return fault(p, "has no member ran")
		}
		t1, err := truth(m, pointer(p, "ran"))
		if err != nil {
			return err
		}
		x.Ran = t1
	}
	{
		m, ok := o["why"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "why"), 0, 2048, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Why = &s
		}
	}
	{
		m, ok := o["findings"]
		if !ok {
			return fault(p, "has no member findings")
		}
		a1, err := array(m, pointer(p, "findings"), 0, 100000)
		if err != nil {
			return err
		}
		x.Findings = make([]Finding, len(a1))
		for i1, e1 := range a1 {
			if err := x.Findings[i1].read(e1, index(pointer(p, "findings"), i1)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Aspect is one of the strings value, options lists.
type Aspect string

const (
	// AspectValue is "value".
	AspectValue Aspect = "value"
	// AspectOptions is "options".
	AspectOptions Aspect = "options"
)

// Valid says whether Aspect is one of the values.
func (e Aspect) Valid() bool {
	switch e {
	case AspectValue, AspectOptions:
		return true
	}
	return false
}

// Pair is a message or part of one of plugin contract 1.
type Pair struct {
	Key    string      `json:"key"`
	Aspect Aspect      `json:"aspect"`
	Preset canon.Value `json:"preset"`
	Found  canon.Value `json:"found"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Pair) ToCanon() canon.Value {
	o := canon.Object{}
	o["key"] = canon.String(x.Key)
	o["aspect"] = canon.String(x.Aspect)
	o["preset"] = x.Preset
	o["found"] = x.Found
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Pair) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Pair) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["key"]
		if !ok {
			return fault(p, "has no member key")
		}
		s1, err := text(m, pointer(p, "key"), 1, 512, nil)
		if err != nil {
			return err
		}
		x.Key = s1
	}
	{
		m, ok := o["aspect"]
		if !ok {
			return fault(p, "has no member aspect")
		}
		s1, err := text(m, pointer(p, "aspect"), 0, -1, nil)
		if err != nil {
			return err
		}
		if !Aspect(s1).Valid() {
			return fault(pointer(p, "aspect"), "is not one of value, options")
		}
		x.Aspect = Aspect(s1)
	}
	{
		m, ok := o["preset"]
		if !ok {
			return fault(p, "has no member preset")
		}
		x.Preset = m
	}
	{
		m, ok := o["found"]
		if !ok {
			return fault(p, "has no member found")
		}
		x.Found = m
	}
	return nil
}

// RankRequest is a message or part of one of plugin contract 1.
type RankRequest struct {
	ToolVersion string `json:"tool_version"`
	Pairs       []Pair `json:"pairs"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x RankRequest) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool_version"] = canon.String(x.ToolVersion)
	o["pairs"] = listOf(x.Pairs, func(x0 Pair) canon.Value { return x0.ToCanon() })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *RankRequest) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *RankRequest) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["tool_version"]
		if !ok {
			return fault(p, "has no member tool_version")
		}
		s1, err := text(m, pointer(p, "tool_version"), 0, 256, pattern0)
		if err != nil {
			return err
		}
		x.ToolVersion = s1
	}
	{
		m, ok := o["pairs"]
		if !ok {
			return fault(p, "has no member pairs")
		}
		a1, err := array(m, pointer(p, "pairs"), 1, 10000)
		if err != nil {
			return err
		}
		x.Pairs = make([]Pair, len(a1))
		for i1, e1 := range a1 {
			if err := x.Pairs[i1].read(e1, index(pointer(p, "pairs"), i1)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Word is one of the strings tighter, weaker, same, unknown lists.
type Word string

const (
	// WordTighter is "tighter".
	WordTighter Word = "tighter"
	// WordWeaker is "weaker".
	WordWeaker Word = "weaker"
	// WordSame is "same".
	WordSame Word = "same"
	// WordUnknown is "unknown".
	WordUnknown Word = "unknown"
)

// Valid says whether Word is one of the values.
func (e Word) Valid() bool {
	switch e {
	case WordTighter, WordWeaker, WordSame, WordUnknown:
		return true
	}
	return false
}

// RankResponse is a message or part of one of plugin contract 1.
type RankResponse struct {
	Words []Word `json:"words"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x RankResponse) ToCanon() canon.Value {
	o := canon.Object{}
	o["words"] = listOf(x.Words, func(x0 Word) canon.Value { return canon.String(x0) })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *RankResponse) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *RankResponse) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "words"); err != nil {
		return err
	}
	{
		m, ok := o["words"]
		if !ok {
			return fault(p, "has no member words")
		}
		a1, err := array(m, pointer(p, "words"), 0, 10000)
		if err != nil {
			return err
		}
		x.Words = make([]Word, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "words"), i1), 0, -1, nil)
			if err != nil {
				return err
			}
			if !Word(s2).Valid() {
				return fault(index(pointer(p, "words"), i1), "is not one of tighter, weaker, same, unknown")
			}
			x.Words[i1] = Word(s2)
		}
	}
	return nil
}

// PresetEntry is a message or part of one of plugin contract 1.
type PresetEntry struct {
	Key      string      `json:"key"`
	Target   Target      `json:"target"`
	Severity *Severity   `json:"severity"`
	Value    canon.Value `json:"value"`
	Options  canon.Value `json:"options"`
	Trial    bool        `json:"trial"`
	Rules    []string    `json:"rules"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x PresetEntry) ToCanon() canon.Value {
	o := canon.Object{}
	o["key"] = canon.String(x.Key)
	o["target"] = canon.String(x.Target)
	if x.Severity != nil {
		o["severity"] = canon.String((*x.Severity))
	}
	if x.Value != nil {
		o["value"] = x.Value
	}
	if x.Options != nil {
		o["options"] = x.Options
	}
	o["trial"] = canon.Bool(x.Trial)
	o["rules"] = listOf(x.Rules, func(x0 string) canon.Value { return canon.String(x0) })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *PresetEntry) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *PresetEntry) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["key"]
		if !ok {
			return fault(p, "has no member key")
		}
		s1, err := text(m, pointer(p, "key"), 1, 512, nil)
		if err != nil {
			return err
		}
		x.Key = s1
	}
	{
		m, ok := o["target"]
		if !ok {
			return fault(p, "has no member target")
		}
		s1, err := text(m, pointer(p, "target"), 0, -1, nil)
		if err != nil {
			return err
		}
		if !Target(s1).Valid() {
			return fault(pointer(p, "target"), "is not one of source, config, migration, container, workflow, iac, api-spec, prose, commit, telemetry-registry, dependency, package")
		}
		x.Target = Target(s1)
	}
	{
		m, ok := o["severity"]
		if ok {
			var s Severity
			s1, err := text(m, pointer(p, "severity"), 0, -1, nil)
			if err != nil {
				return err
			}
			if !Severity(s1).Valid() {
				return fault(pointer(p, "severity"), "is not one of error, warning, info")
			}
			s = Severity(s1)
			x.Severity = &s
		}
	}
	{
		m, ok := o["value"]
		if ok {
			x.Value = m
		}
	}
	{
		m, ok := o["options"]
		if ok {
			x.Options = m
		}
	}
	{
		m, ok := o["trial"]
		if !ok {
			return fault(p, "has no member trial")
		}
		t1, err := truth(m, pointer(p, "trial"))
		if err != nil {
			return err
		}
		x.Trial = t1
	}
	{
		m, ok := o["rules"]
		if !ok {
			return fault(p, "has no member rules")
		}
		a1, err := array(m, pointer(p, "rules"), 1, 4096)
		if err != nil {
			return err
		}
		x.Rules = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "rules"), i1), 0, 128, nil)
			if err != nil {
				return err
			}
			x.Rules[i1] = s2
		}
	}
	return nil
}

// PresetRequest is a message or part of one of plugin contract 1.
type PresetRequest struct {
	ToolVersion string        `json:"tool_version"`
	Mark        *string       `json:"mark"`
	Inputs      []File        `json:"inputs"`
	Entries     []PresetEntry `json:"entries"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x PresetRequest) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool_version"] = canon.String(x.ToolVersion)
	if x.Mark != nil {
		o["mark"] = canon.String((*x.Mark))
	}
	o["inputs"] = listOf(x.Inputs, func(x0 File) canon.Value { return x0.ToCanon() })
	o["entries"] = listOf(x.Entries, func(x0 PresetEntry) canon.Value { return x0.ToCanon() })
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *PresetRequest) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *PresetRequest) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["tool_version"]
		if !ok {
			return fault(p, "has no member tool_version")
		}
		s1, err := text(m, pointer(p, "tool_version"), 0, 256, pattern0)
		if err != nil {
			return err
		}
		x.ToolVersion = s1
	}
	{
		m, ok := o["mark"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "mark"), 0, 256, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Mark = &s
		}
	}
	{
		m, ok := o["inputs"]
		if !ok {
			return fault(p, "has no member inputs")
		}
		a1, err := array(m, pointer(p, "inputs"), 0, 16)
		if err != nil {
			return err
		}
		x.Inputs = make([]File, len(a1))
		for i1, e1 := range a1 {
			if err := x.Inputs[i1].read(e1, index(pointer(p, "inputs"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["entries"]
		if !ok {
			return fault(p, "has no member entries")
		}
		a1, err := array(m, pointer(p, "entries"), 0, 100000)
		if err != nil {
			return err
		}
		x.Entries = make([]PresetEntry, len(a1))
		for i1, e1 := range a1 {
			if err := x.Entries[i1].read(e1, index(pointer(p, "entries"), i1)); err != nil {
				return err
			}
		}
	}
	return nil
}

// PresetFile is a message or part of one of plugin contract 1.
type PresetFile struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x PresetFile) ToCanon() canon.Value {
	o := canon.Object{}
	o["path"] = canon.String(x.Path)
	o["text"] = canon.String(x.Text)
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *PresetFile) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *PresetFile) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "path", "text"); err != nil {
		return err
	}
	{
		m, ok := o["path"]
		if !ok {
			return fault(p, "has no member path")
		}
		s1, err := text(m, pointer(p, "path"), 1, 4096, nil)
		if err != nil {
			return err
		}
		x.Path = s1
	}
	{
		m, ok := o["text"]
		if !ok {
			return fault(p, "has no member text")
		}
		s1, err := text(m, pointer(p, "text"), 0, -1, nil)
		if err != nil {
			return err
		}
		x.Text = s1
	}
	return nil
}

// PresetResponse is a message or part of one of plugin contract 1.
type PresetResponse struct {
	Files []PresetFile `json:"files"`
	Notes []string     `json:"notes"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x PresetResponse) ToCanon() canon.Value {
	o := canon.Object{}
	o["files"] = listOf(x.Files, func(x0 PresetFile) canon.Value { return x0.ToCanon() })
	if x.Notes != nil {
		o["notes"] = listOf(x.Notes, func(x0 string) canon.Value { return canon.String(x0) })
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *PresetResponse) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *PresetResponse) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "files", "notes"); err != nil {
		return err
	}
	{
		m, ok := o["files"]
		if !ok {
			return fault(p, "has no member files")
		}
		a1, err := array(m, pointer(p, "files"), 1, 10000)
		if err != nil {
			return err
		}
		x.Files = make([]PresetFile, len(a1))
		for i1, e1 := range a1 {
			if err := x.Files[i1].read(e1, index(pointer(p, "files"), i1)); err != nil {
				return err
			}
		}
	}
	{
		m, ok := o["notes"]
		if ok {
			a1, err := array(m, pointer(p, "notes"), 0, 100)
			if err != nil {
				return err
			}
			x.Notes = make([]string, len(a1))
			for i1, e1 := range a1 {
				s2, err := text(e1, index(pointer(p, "notes"), i1), 0, 1024, nil)
				if err != nil {
					return err
				}
				x.Notes[i1] = s2
			}
		}
	}
	return nil
}

// Code is one of the strings unsupported, unreadable, refused, bug lists.
type Code string

const (
	// CodeUnsupported is "unsupported".
	CodeUnsupported Code = "unsupported"
	// CodeUnreadable is "unreadable".
	CodeUnreadable Code = "unreadable"
	// CodeRefused is "refused".
	CodeRefused Code = "refused"
	// CodeBug is "bug".
	CodeBug Code = "bug"
)

// Valid says whether Code is one of the values.
func (e Code) Valid() bool {
	switch e {
	case CodeUnsupported, CodeUnreadable, CodeRefused, CodeBug:
		return true
	}
	return false
}

// Error is a message or part of one of plugin contract 1.
type Error struct {
	Code    Code    `json:"code"`
	Message string  `json:"message"`
	Path    *string `json:"path"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Error) ToCanon() canon.Value {
	o := canon.Object{}
	o["code"] = canon.String(x.Code)
	o["message"] = canon.String(x.Message)
	if x.Path != nil {
		o["path"] = canon.String((*x.Path))
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Error) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Error) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "code", "message", "path"); err != nil {
		return err
	}
	{
		m, ok := o["code"]
		if !ok {
			return fault(p, "has no member code")
		}
		s1, err := text(m, pointer(p, "code"), 0, -1, nil)
		if err != nil {
			return err
		}
		if !Code(s1).Valid() {
			return fault(pointer(p, "code"), "is not one of unsupported, unreadable, refused, bug")
		}
		x.Code = Code(s1)
	}
	{
		m, ok := o["message"]
		if !ok {
			return fault(p, "has no member message")
		}
		s1, err := text(m, pointer(p, "message"), 1, 2048, nil)
		if err != nil {
			return err
		}
		x.Message = s1
	}
	{
		m, ok := o["path"]
		if ok {
			var s string
			s1, err := text(m, pointer(p, "path"), 0, 1024, nil)
			if err != nil {
				return err
			}
			s = s1
			x.Path = &s
		}
	}
	return nil
}

// Op is one of the strings plan, entries, findings, rank, preset lists.
type Op string

const (
	// OpPlan is "plan".
	OpPlan Op = "plan"
	// OpEntries is "entries".
	OpEntries Op = "entries"
	// OpFindings is "findings".
	OpFindings Op = "findings"
	// OpRank is "rank".
	OpRank Op = "rank"
	// OpPreset is "preset".
	OpPreset Op = "preset"
)

// Valid says whether Op is one of the values.
func (e Op) Valid() bool {
	switch e {
	case OpPlan, OpEntries, OpFindings, OpRank, OpPreset:
		return true
	}
	return false
}

// Request is a message or part of one of plugin contract 1.
type Request struct {
	Features []string     `json:"features"`
	Op       Op           `json:"op"`
	Body     canon.Object `json:"body"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Request) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = canon.Int(1)
	o["features"] = listOf(x.Features, func(x0 string) canon.Value { return canon.String(x0) })
	o["op"] = canon.String(x.Op)
	o["body"] = x.Body
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Request) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Request) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	{
		m, ok := o["contract"]
		if !ok {
			return fault(p, "has no member contract")
		}
		if m != canon.Int(1) {
			return fault(pointer(p, "contract"), "is not 1")
		}
	}
	{
		m, ok := o["features"]
		if !ok {
			return fault(p, "has no member features")
		}
		a1, err := array(m, pointer(p, "features"), 0, 4096)
		if err != nil {
			return err
		}
		x.Features = make([]string, len(a1))
		for i1, e1 := range a1 {
			s2, err := text(e1, index(pointer(p, "features"), i1), 0, -1, nil)
			if err != nil {
				return err
			}
			x.Features[i1] = s2
		}
	}
	{
		m, ok := o["op"]
		if !ok {
			return fault(p, "has no member op")
		}
		s1, err := text(m, pointer(p, "op"), 0, -1, nil)
		if err != nil {
			return err
		}
		if !Op(s1).Valid() {
			return fault(pointer(p, "op"), "is not one of plan, entries, findings, rank, preset")
		}
		x.Op = Op(s1)
	}
	{
		m, ok := o["body"]
		if !ok {
			return fault(p, "has no member body")
		}
		o1, err := object(m, pointer(p, "body"))
		if err != nil {
			return err
		}
		x.Body = o1
	}
	return nil
}

// Response is a message or part of one of plugin contract 1.
type Response struct {
	Ok    canon.Object `json:"ok"`
	Error *Error       `json:"error"`
}

// ToCanon is x as canon's value tree: a member that is optional and not set is
// left out.
func (x Response) ToCanon() canon.Value {
	o := canon.Object{}
	if x.Ok != nil {
		o["ok"] = x.Ok
	}
	if x.Error != nil {
		o["error"] = (*x.Error).ToCanon()
	}
	return o
}

// FromCanon reads v, a document that passed canon.Decode, into x, and refuses
// what the schema refuses that a type can hold: a member of another type, a
// null, a member the object does not have when it is closed, a member it
// requires and lacks, a value outside an enum, a pattern or a bound. The
// *Fault names where, by JSON Pointer.
func (x *Response) FromCanon(v canon.Value) error { return x.read(v, "") }

func (x *Response) read(v canon.Value, p string) error {
	o, err := object(v, p)
	if err != nil {
		return err
	}
	if err := known(o, p, "ok", "error"); err != nil {
		return err
	}
	if err := exactlyOne(o, p, "ok", "error"); err != nil {
		return err
	}
	{
		m, ok := o["ok"]
		if ok {
			o1, err := object(m, pointer(p, "ok"))
			if err != nil {
				return err
			}
			x.Ok = o1
		}
	}
	{
		m, ok := o["error"]
		if ok {
			var s Error
			if err := s.read(m, pointer(p, "error")); err != nil {
				return err
			}
			x.Error = &s
		}
	}
	return nil
}
