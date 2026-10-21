// Package schema is a disposable Phase 1 schema/parser proof harness, not product code.
package schema

const ResultVersion = 3

const (
	MaxDocumentBytes   = 1 << 20
	MaxProfiles        = 256
	MaxProfileIDBytes  = 64
	MaxProfileLabel    = 256
	MaxSourceBytes     = 2048
	MaxPathBytes       = 1024
	MaxPathComponents  = 64
	MaxPathComponent   = 255
	MaxComponentFiles  = 4096
	MaxTools           = 1
	MaxToolIDBytes     = 4
	MaxVersionBytes    = 6
	MaxLocksPerTool    = 1
	MaxLockValueBytes  = 71
	MaxHomeSources     = 4096
	MaxTargets         = 4096
	MaxTemplateBytes   = 1 << 20
	MaxTemplateDepth   = 32
	MaxTemplateActions = 4096
	MaxTotalStrings    = 16384
	MaxAllocatedBytes  = 8_000_000
)

type Config struct {
	Root           string   `json:"root"`
	ArtifactDir    string   `json:"artifact_dir"`
	ProtectedRoots []string `json:"-"`
}

type ZerbManifest struct {
	Schema   int             `toml:"schema" json:"schema"`
	Base     BaseDeclaration `toml:"base" json:"base"`
	Profiles []Profile       `toml:"profiles" json:"profiles"`
}

type BaseDeclaration struct {
	Path string `toml:"path" json:"path"`
}

type Profile struct {
	ID   string `toml:"id" json:"id"`
	Kind string `toml:"kind" json:"kind"`
	Path string `toml:"path" json:"path"`
}

type ComponentManifest struct {
	Schema int    `toml:"schema" json:"schema"`
	Label  string `toml:"label,omitempty" json:"label,omitempty"`
}

type Result struct {
	Version              int                     `json:"version"`
	Parser               ParserIdentity          `json:"parser"`
	Requirements         []string                `json:"requirements"`
	Bounds               []Bound                 `json:"bounds"`
	Scenarios            []Scenario              `json:"scenarios"`
	Diagnostics          []DiagnosticObservation `json:"diagnostic_observations"`
	RejectedAssumption   string                  `json:"rejected_assumption"`
	RenderedFingerprint  string                  `json:"rendered_marker_fingerprint"`
	AmbientFingerprint   string                  `json:"ambient_marker_fingerprint"`
	SerializedMarkerScan MarkerScan              `json:"serialized_marker_scan"`
	EvidenceRelations    []EvidenceRelation      `json:"evidence_relations"`
}

type ParserIdentity struct {
	Module       string `json:"module"`
	Version      string `json:"version"`
	ModuleSum    string `json:"module_sum"`
	GoModSum     string `json:"go_mod_sum"`
	OriginCommit string `json:"origin_commit"`
}

type Bound struct {
	Field     string `json:"field"`
	Limit     int    `json:"limit"`
	Unit      string `json:"unit"`
	Rationale string `json:"rationale"`
}

type Scenario struct {
	Name               string   `json:"name"`
	Status             string   `json:"status"`
	RequirementIDs     []string `json:"requirement_ids"`
	Expected           string   `json:"expected"`
	Observed           string   `json:"observed"`
	RejectedAssumption string   `json:"rejected_assumption"`
}

type DiagnosticObservation struct {
	Case    string `json:"case"`
	Message string `json:"message"`
	Row     int    `json:"row"`
	Column  int    `json:"column"`
	Key     string `json:"key"`
}

type MarkerScan struct {
	RenderedRawOccurrences int `json:"rendered_raw_occurrences"`
	AmbientRawOccurrences  int `json:"ambient_raw_occurrences"`
}

type EvidenceRelation struct {
	LeftRef  string `json:"left_ref"`
	Relation string `json:"relation"`
	RightRef string `json:"right_ref"`
}

type GitlinkFact struct {
	Path       string
	Locator    string
	TreeObject string
}

type Component struct {
	ID          string
	Path        string
	TreeDigest  string
	Manifest    ComponentManifest
	HasMise     bool
	HasLock     bool
	HasHome     bool
	Tools       []Tool
	HomeSources []string
	Targets     []string
	Files       []CanonicalFile
}

type Tool struct {
	ID      string
	Version string
	Locks   []ToolLock
}

type ToolLock struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type CanonicalFile struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Hash string `json:"hash"`
}

type CandidatePreimage struct {
	Schema      int                   `json:"schema"`
	Domain      string                `json:"domain"`
	Source      CanonicalSource       `json:"source"`
	ProfileID   string                `json:"profile_id"`
	Components  []CanonicalComponent  `json:"components"`
	Tools       []CanonicalTool       `json:"tools"`
	HomeSources []string              `json:"home_sources"`
	Targets     []string              `json:"targets"`
	Executables []CanonicalExecutable `json:"executables"`
	Parser      CanonicalParser       `json:"parser"`
	Policy      CanonicalPolicy       `json:"policy"`
}

type CanonicalSource struct {
	URI      string `json:"uri"`
	SourceID string `json:"source_id"`
	Commit   string `json:"commit"`
	RootTree string `json:"root_tree"`
}

type CanonicalComponent struct {
	ID         string          `json:"id"`
	Path       string          `json:"path"`
	TreeDigest string          `json:"tree_digest"`
	Files      []CanonicalFile `json:"files"`
}

type CanonicalTool struct {
	ID      string     `json:"id"`
	Version string     `json:"version"`
	Locks   []ToolLock `json:"locks"`
}

type CanonicalExecutable struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
}

type CanonicalParser struct {
	Schema       int    `json:"schema"`
	Module       string `json:"module"`
	Version      string `json:"version"`
	ModuleSum    string `json:"module_sum"`
	GoModSum     string `json:"go_mod_sum"`
	OriginCommit string `json:"origin_commit"`
}

type CanonicalPolicy struct {
	Schema                   int      `json:"schema"`
	Domain                   string   `json:"domain"`
	ObjectModes              []string `json:"object_modes"`
	Declarations             []string `json:"declarations"`
	TemplateActions          []string `json:"template_actions"`
	ProcessClasses           []string `json:"process_classes"`
	PolicyIDs                []string `json:"policy_ids"`
	TargetMapperExecutableID string   `json:"target_mapper_executable_id"`
	ProviderFree             bool     `json:"provider_free"`
	HostFactIndependent      bool     `json:"host_fact_independent"`
	DestinationRecheck       bool     `json:"destination_recheck"`
	ExactReapplyOnly         bool     `json:"exact_reapply_only"`
}
