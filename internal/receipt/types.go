// Package receipt canonical value shapes are owned by ARCH-030.
package receipt

type Source struct {
	URI      string `json:"uri"`
	SourceID string `json:"source_id"`
	Commit   string `json:"commit"`
	RootTree string `json:"root_tree"`
}

type File struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Hash string `json:"hash"`
}

type Component struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	TreeDigest string `json:"tree_digest"`
	Files      []File `json:"files"`
}

type LockValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Tool struct {
	ID      string      `json:"id"`
	Version string      `json:"version"`
	Locks   []LockValue `json:"locks"`
}

type Executable struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
}

type Parser struct {
	Schema       int    `json:"schema"`
	Module       string `json:"module"`
	Version      string `json:"version"`
	ModuleSum    string `json:"module_sum"`
	GoModSum     string `json:"go_mod_sum"`
	OriginCommit string `json:"origin_commit"`
}

type Policy struct {
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

type Candidate struct {
	Schema      int          `json:"schema"`
	Domain      string       `json:"domain"`
	Source      Source       `json:"source"`
	ProfileID   string       `json:"profile_id"`
	Components  []Component  `json:"components"`
	Tools       []Tool       `json:"tools"`
	HomeSources []string     `json:"home_sources"`
	Targets     []string     `json:"targets"`
	Executables []Executable `json:"executables"`
	Parser      Parser       `json:"parser"`
	Policy      Policy       `json:"policy"`
}

type DestinationObservation struct {
	Target      string `json:"target"`
	Exists      bool   `json:"exists"`
	Kind        string `json:"kind"`
	Mode        uint32 `json:"mode"`
	Owner       uint32 `json:"owner"`
	Device      uint64 `json:"device"`
	Inode       uint64 `json:"inode"`
	Links       uint64 `json:"links"`
	ContentHash string `json:"content_hash"`
}

type ToolObservation struct {
	ID        string `json:"id"`
	Requested string `json:"requested"`
	Installed string `json:"installed"`
	Integrity string `json:"integrity"`
}

type PlatformObservation struct {
	Distribution string `json:"distribution"`
	Kernel       string `json:"kernel"`
	Architecture string `json:"architecture"`
	Filesystem   string `json:"filesystem"`
}

type Dimensions struct {
	InputsPinned        bool `json:"inputs_pinned"`
	ArtifactIntegrity   bool `json:"artifact_integrity"`
	ArtifactAvailable   bool `json:"artifact_available"`
	HostCompatible      bool `json:"host_compatible"`
	ConvergenceVerified bool `json:"convergence_verified"`
}

type Completed struct {
	Schema        int                      `json:"schema"`
	Domain        string                   `json:"domain"`
	Candidate     Digest                   `json:"candidate"`
	Previous      Digest                   `json:"previous,omitempty"`
	Source        Source                   `json:"source"`
	Components    []Component              `json:"components"`
	Zerb          Executable               `json:"zerb"`
	Git           Executable               `json:"git"`
	Mise          Executable               `json:"mise"`
	Chezmoi       Executable               `json:"chezmoi"`
	Parser        Parser                   `json:"parser"`
	Policy        Policy                   `json:"policy"`
	Platform      PlatformObservation      `json:"platform"`
	Tools         []ToolObservation        `json:"tools"`
	Targets       []string                 `json:"targets"`
	Preconditions []DestinationObservation `json:"preconditions"`
	Results       []DestinationObservation `json:"results"`
	Phases        []string                 `json:"phases"`
	Dimensions    Dimensions               `json:"dimensions"`
}
