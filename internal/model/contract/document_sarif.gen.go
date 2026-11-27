// Written by gragas-generate from internal/host/assets/contract/schema/v1/document-sarif.schema.json; edits are lost, so edit the source and run task gen.

package contract

import "github.com/birdtable/gragas/internal/canon"

// SarifDocument is SARIF 2.1.0 with the engine's extension (document).
type SarifDocument struct {
	// Decides: which contract version's schema reads the document, and whether it is read or ends not ready on a newer major.
	// Since contract 1.0.
	Contract Contract `json:"contract"`
	// Display only.
	// Since contract 1.0.
	// Constraints: always 2.1.0.
	Version string `json:"version"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Schema *string `json:"$schema,omitempty"`
	// Display only.
	// Since contract 1.0.
	Runs []SarifDocumentRuns `json:"runs"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocument.
func (v SarifDocument) ToCanon() canon.Value {
	o := canon.Object{}
	o["contract"] = v.Contract.ToCanon()
	o["version"] = textOf(v.Version)
	keepPtr(o, "$schema", v.Schema, textOf)
	o["runs"] = listOf(v.Runs, SarifDocumentRuns.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocument has no
// field for, under its exact name, is refused.
func (v *SarifDocument) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocument) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "contract", "version", "$schema", "runs")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "contract", &v.Contract, readVia[Contract]),
		field(o, at, "version", &v.Version, readText),
		field(o, at, "$schema", &v.Schema, ptrRead(readText)),
		field(o, at, "runs", &v.Runs, listRead(readVia[SarifDocumentRuns])),
	)
}

// SarifDocumentRunsToolDriverRulesShortDescription is has the shape the schema gives it.
type SarifDocumentRunsToolDriverRulesShortDescription struct {
	// Display only.
	// Since contract 1.0.
	Text Text `json:"text"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsToolDriverRulesShortDescription.
func (v SarifDocumentRunsToolDriverRulesShortDescription) ToCanon() canon.Value {
	o := canon.Object{}
	o["text"] = v.Text.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsToolDriverRulesShortDescription has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsToolDriverRulesShortDescription) FromCanon(c canon.Value) error {
	return v.fromAt(c, "")
}

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsToolDriverRulesShortDescription) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "text")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "text", &v.Text, readVia[Text]),
	)
}

// SarifDocumentRunsToolDriverRules is has the shape the schema gives it.
type SarifDocumentRunsToolDriverRules struct {
	// Display only.
	// Since contract 1.0.
	ID ID `json:"id"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	ShortDescription *SarifDocumentRunsToolDriverRulesShortDescription `json:"shortDescription,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsToolDriverRules.
func (v SarifDocumentRunsToolDriverRules) ToCanon() canon.Value {
	o := canon.Object{}
	o["id"] = v.ID.ToCanon()
	keepPtr(o, "shortDescription", v.ShortDescription, SarifDocumentRunsToolDriverRulesShortDescription.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsToolDriverRules has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsToolDriverRules) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsToolDriverRules) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "id", "shortDescription")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "id", &v.ID, readVia[ID]),
		field(o, at, "shortDescription", &v.ShortDescription, ptrRead(readVia[SarifDocumentRunsToolDriverRulesShortDescription])),
	)
}

// SarifDocumentRunsToolDriver is has the shape the schema gives it.
type SarifDocumentRunsToolDriver struct {
	// Display only.
	// Since contract 1.0.
	Name Text `json:"name"`
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Version *Semver `json:"version,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Rules []SarifDocumentRunsToolDriverRules `json:"rules,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsToolDriver.
func (v SarifDocumentRunsToolDriver) ToCanon() canon.Value {
	o := canon.Object{}
	o["name"] = v.Name.ToCanon()
	keepPtr(o, "version", v.Version, Semver.ToCanon)
	keep(o, "rules", listOf(v.Rules, SarifDocumentRunsToolDriverRules.ToCanon), len(v.Rules) > 0)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsToolDriver has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsToolDriver) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsToolDriver) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "name", "version", "rules")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "name", &v.Name, readVia[Text]),
		field(o, at, "version", &v.Version, ptrRead(readVia[Semver])),
		field(o, at, "rules", &v.Rules, listRead(readVia[SarifDocumentRunsToolDriverRules])),
	)
}

// SarifDocumentRunsTool is has the shape the schema gives it.
type SarifDocumentRunsTool struct {
	// Display only.
	// Since contract 1.0.
	Driver SarifDocumentRunsToolDriver `json:"driver"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsTool.
func (v SarifDocumentRunsTool) ToCanon() canon.Value {
	o := canon.Object{}
	o["driver"] = v.Driver.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsTool has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsTool) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsTool) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "driver")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "driver", &v.Driver, readVia[SarifDocumentRunsToolDriver]),
	)
}

// SarifDocumentRunsResultsLevel is has the shape the schema gives it.
type SarifDocumentRunsResultsLevel string

// The values of SarifDocumentRunsResultsLevel.
const (
	SarifDocumentRunsResultsLevelError   SarifDocumentRunsResultsLevel = "error"
	SarifDocumentRunsResultsLevelWarning SarifDocumentRunsResultsLevel = "warning"
	SarifDocumentRunsResultsLevelNote    SarifDocumentRunsResultsLevel = "note"
	SarifDocumentRunsResultsLevelNone    SarifDocumentRunsResultsLevel = "none"
)

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsResultsLevel.
func (v SarifDocumentRunsResultsLevel) ToCanon() canon.Value {
	return textOf(string(v))
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsResultsLevel has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsResultsLevel) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsResultsLevel) fromAt(c canon.Value, at string) error {
	x, err := readText(c, at)
	*v = SarifDocumentRunsResultsLevel(x)
	return err
}

// SarifDocumentRunsResultsMessage is has the shape the schema gives it.
type SarifDocumentRunsResultsMessage struct {
	// Display only.
	// Since contract 1.0.
	Text Text `json:"text"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsResultsMessage.
func (v SarifDocumentRunsResultsMessage) ToCanon() canon.Value {
	o := canon.Object{}
	o["text"] = v.Text.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsResultsMessage has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsResultsMessage) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsResultsMessage) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "text")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "text", &v.Text, readVia[Text]),
	)
}

// SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation is has the shape the schema gives it.
type SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation struct {
	// Display only.
	// Since contract 1.0.
	URI Text `json:"uri"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation.
func (v SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation) ToCanon() canon.Value {
	o := canon.Object{}
	o["uri"] = v.URI.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation) FromCanon(c canon.Value) error {
	return v.fromAt(c, "")
}

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "uri")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "uri", &v.URI, readVia[Text]),
	)
}

// SarifDocumentRunsResultsLocationsPhysicalLocationRegion is has the shape the schema gives it.
type SarifDocumentRunsResultsLocationsPhysicalLocationRegion struct {
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	// Constraints: at least 1.
	StartLine *int `json:"startLine,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsResultsLocationsPhysicalLocationRegion.
func (v SarifDocumentRunsResultsLocationsPhysicalLocationRegion) ToCanon() canon.Value {
	o := canon.Object{}
	keepPtr(o, "startLine", v.StartLine, wholeOf)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsResultsLocationsPhysicalLocationRegion has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsResultsLocationsPhysicalLocationRegion) FromCanon(c canon.Value) error {
	return v.fromAt(c, "")
}

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsResultsLocationsPhysicalLocationRegion) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "startLine")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "startLine", &v.StartLine, ptrRead(readWhole)),
	)
}

// SarifDocumentRunsResultsLocationsPhysicalLocation is has the shape the schema gives it.
type SarifDocumentRunsResultsLocationsPhysicalLocation struct {
	// Display only.
	// Since contract 1.0.
	ArtifactLocation SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation `json:"artifactLocation"`
	// Display only.
	// Since contract 1.0.
	// Absent means {}.
	Region *SarifDocumentRunsResultsLocationsPhysicalLocationRegion `json:"region,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsResultsLocationsPhysicalLocation.
func (v SarifDocumentRunsResultsLocationsPhysicalLocation) ToCanon() canon.Value {
	o := canon.Object{}
	o["artifactLocation"] = v.ArtifactLocation.ToCanon()
	keepPtr(o, "region", v.Region, SarifDocumentRunsResultsLocationsPhysicalLocationRegion.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsResultsLocationsPhysicalLocation has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsResultsLocationsPhysicalLocation) FromCanon(c canon.Value) error {
	return v.fromAt(c, "")
}

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsResultsLocationsPhysicalLocation) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "artifactLocation", "region")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "artifactLocation", &v.ArtifactLocation, readVia[SarifDocumentRunsResultsLocationsPhysicalLocationArtifactLocation]),
		field(o, at, "region", &v.Region, ptrRead(readVia[SarifDocumentRunsResultsLocationsPhysicalLocationRegion])),
	)
}

// SarifDocumentRunsResultsLocations is has the shape the schema gives it.
type SarifDocumentRunsResultsLocations struct {
	// Display only.
	// Since contract 1.0.
	PhysicalLocation SarifDocumentRunsResultsLocationsPhysicalLocation `json:"physicalLocation"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsResultsLocations.
func (v SarifDocumentRunsResultsLocations) ToCanon() canon.Value {
	o := canon.Object{}
	o["physicalLocation"] = v.PhysicalLocation.ToCanon()
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsResultsLocations has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsResultsLocations) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsResultsLocations) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "physicalLocation")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "physicalLocation", &v.PhysicalLocation, readVia[SarifDocumentRunsResultsLocationsPhysicalLocation]),
	)
}

// SarifDocumentRunsResultsPropertiesGragas is has the shape the schema gives it.
type SarifDocumentRunsResultsPropertiesGragas struct {
	// null means not applicable: the result is not about a rule.
	// Display only.
	// Since contract 1.0.
	// Absent means null.
	Standing *Standing `json:"standing,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means "".
	Fix TextOrEmpty `json:"fix,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means false.
	Notice bool `json:"notice,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsResultsPropertiesGragas.
func (v SarifDocumentRunsResultsPropertiesGragas) ToCanon() canon.Value {
	o := canon.Object{}
	keepPtr(o, "standing", v.Standing, Standing.ToCanon)
	keep(o, "fix", v.Fix.ToCanon(), v.Fix != "")
	keep(o, "notice", truthOf(v.Notice), v.Notice)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsResultsPropertiesGragas has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsResultsPropertiesGragas) FromCanon(c canon.Value) error {
	return v.fromAt(c, "")
}

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsResultsPropertiesGragas) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "standing", "fix", "notice")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "standing", &v.Standing, ptrRead(readVia[Standing])),
		field(o, at, "fix", &v.Fix, readVia[TextOrEmpty]),
		field(o, at, "notice", &v.Notice, readTruth),
	)
}

// SarifDocumentRunsResultsProperties is has the shape the schema gives it.
type SarifDocumentRunsResultsProperties struct {
	// Display only.
	// Since contract 1.0.
	// Absent means {}.
	Gragas *SarifDocumentRunsResultsPropertiesGragas `json:"gragas,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsResultsProperties.
func (v SarifDocumentRunsResultsProperties) ToCanon() canon.Value {
	o := canon.Object{}
	keepPtr(o, "gragas", v.Gragas, SarifDocumentRunsResultsPropertiesGragas.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsResultsProperties has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsResultsProperties) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsResultsProperties) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "gragas")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "gragas", &v.Gragas, ptrRead(readVia[SarifDocumentRunsResultsPropertiesGragas])),
	)
}

// SarifDocumentRunsResults is has the shape the schema gives it.
type SarifDocumentRunsResults struct {
	// Display only.
	// Since contract 1.0.
	RuleID ID `json:"ruleId"`
	// Display only.
	// Since contract 1.0.
	// Absent means "error".
	Level *SarifDocumentRunsResultsLevel `json:"level,omitempty"`
	// Display only.
	// Since contract 1.0.
	Message SarifDocumentRunsResultsMessage `json:"message"`
	// Display only.
	// Since contract 1.0.
	// Absent means [].
	Locations []SarifDocumentRunsResultsLocations `json:"locations,omitempty"`
	// Display only.
	// Since contract 1.0.
	// Absent means {}.
	Properties *SarifDocumentRunsResultsProperties `json:"properties,omitempty"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRunsResults.
func (v SarifDocumentRunsResults) ToCanon() canon.Value {
	o := canon.Object{}
	o["ruleId"] = v.RuleID.ToCanon()
	keepPtr(o, "level", v.Level, SarifDocumentRunsResultsLevel.ToCanon)
	o["message"] = v.Message.ToCanon()
	keep(o, "locations", listOf(v.Locations, SarifDocumentRunsResultsLocations.ToCanon), len(v.Locations) > 0)
	keepPtr(o, "properties", v.Properties, SarifDocumentRunsResultsProperties.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRunsResults has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRunsResults) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRunsResults) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "ruleId", "level", "message", "locations", "properties")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "ruleId", &v.RuleID, readVia[ID]),
		field(o, at, "level", &v.Level, ptrRead(readVia[SarifDocumentRunsResultsLevel])),
		field(o, at, "message", &v.Message, readVia[SarifDocumentRunsResultsMessage]),
		field(o, at, "locations", &v.Locations, listRead(readVia[SarifDocumentRunsResultsLocations])),
		field(o, at, "properties", &v.Properties, ptrRead(readVia[SarifDocumentRunsResultsProperties])),
	)
}

// SarifDocumentRuns is has the shape the schema gives it.
type SarifDocumentRuns struct {
	// Display only.
	// Since contract 1.0.
	Tool SarifDocumentRunsTool `json:"tool"`
	// Display only.
	// Since contract 1.0.
	Results []SarifDocumentRunsResults `json:"results"`
}

// ToCanon is v as canon's value tree, as encoding/json wrote SarifDocumentRuns.
func (v SarifDocumentRuns) ToCanon() canon.Value {
	o := canon.Object{}
	o["tool"] = v.Tool.ToCanon()
	o["results"] = listOf(v.Results, SarifDocumentRunsResults.ToCanon)
	return o
}

// FromCanon reads c, a value canon decoded, into v: a member SarifDocumentRuns has no
// field for, under its exact name, is refused.
func (v *SarifDocumentRuns) FromCanon(c canon.Value) error { return v.fromAt(c, "") }

// fromAt is FromCanon of the value at the JSON Pointer at.
func (v *SarifDocumentRuns) fromAt(c canon.Value, at string) error {
	o, err := objectAt(c, at, "tool", "results")
	if o == nil {
		return err
	}
	return first(
		field(o, at, "tool", &v.Tool, readVia[SarifDocumentRunsTool]),
		field(o, at, "results", &v.Results, listRead(readVia[SarifDocumentRunsResults])),
	)
}
