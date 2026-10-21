package git

import "time"

const (
	ResultVersion          = "zerb.git-proof.v3"
	PinnedGitVersion       = "2.55.0"
	MaxDiskBytes     int64 = 32 << 20
	MaxObjectBytes   int64 = 1 << 20
	MaxExpandedBytes int64 = 8 << 20
	MaxObjectCount         = 256
	MaxGraphDepth          = 32
	MaxTreeDepth           = 16
	MaxTreeEntries         = 256
	MaxProcessOutput int64 = 1 << 20
	MaxWallTime            = 30 * time.Second
	MaxProcessCount        = 64
)

type Config struct {
	Root             string
	RepositoryRoot   string
	GitPath          string
	TracePath        string
	SupervisorPath   string
	CACert           string
	ArtifactDir      string
	Source           string
	ProtectedRoots   []string
	DelegatedHostUID int
}

type Result struct {
	Version                  string              `json:"version"`
	StartedAt                time.Time           `json:"started_at"`
	FinishedAt               time.Time           `json:"finished_at"`
	CandidateRows            []string            `json:"candidate_rows"`
	Requirements             []string            `json:"requirements"`
	AmbientMarkerFingerprint string              `json:"ambient_marker_fingerprint"`
	Source                   SourceObservation   `json:"source"`
	Executable               FileIdentity        `json:"executable"`
	Roots                    []RootObservation   `json:"roots"`
	Scenarios                []Scenario          `json:"scenarios"`
	Invocations              []Invocation        `json:"invocations"`
	Access                   AccessObservation   `json:"access"`
	Writes                   WriteObservation    `json:"writes"`
	Resources                ResourceObservation `json:"resources"`
	Trees                    TreeObservation     `json:"trees"`
	Cleanup                  CleanupObservation  `json:"cleanup"`
	RejectedAssumption       string              `json:"rejected_assumption"`
}

type Scenario struct {
	Name               string   `json:"name"`
	Status             string   `json:"status"`
	RequirementIDs     []string `json:"requirement_ids"`
	Observation        string   `json:"observation"`
	RejectedAssumption string   `json:"rejected_assumption,omitempty"`
}

type SourceObservation struct {
	ReviewedCanonicalHTTPS string `json:"reviewed_canonical_https"`
	SourceID               string `json:"source_id"`
	LocalTransport         string `json:"local_transport"`
	IdentitySeparated      bool   `json:"identity_separated"`
}

type FileIdentity struct {
	CanonicalPath   string `json:"canonical_path"`
	UpstreamVersion string `json:"upstream_version"`
	Device          uint64 `json:"device"`
	Inode           uint64 `json:"inode"`
	Mode            string `json:"mode"`
	Size            int64  `json:"size"`
}

type RootObservation struct {
	Role          string `json:"role"`
	CanonicalPath string `json:"canonical_path"`
	Device        uint64 `json:"device"`
	Inode         uint64 `json:"inode"`
	Mode          string `json:"mode"`
}

type Invocation struct {
	Executable          string   `json:"executable"`
	Argv                []string `json:"argv"`
	EnvironmentNames    []string `json:"environment_names"`
	WorkingDirectoryRef string   `json:"working_directory_ref"`
	ExitCode            int      `json:"exit_code"`
	OutputBytes         int64    `json:"output_bytes"`
	DurationMillis      int64    `json:"duration_millis"`
	ObservedExecs       []string `json:"observed_execs"`
	TraceComplete       bool     `json:"trace_complete"`
}

type AccessObservation struct {
	Instrumentation       string   `json:"instrumentation"`
	Endpoints             []string `json:"endpoints"`
	GitlinkEndpoints      int      `json:"gitlink_endpoints"`
	HelperExecs           int      `json:"helper_execs"`
	AgentAccesses         int      `json:"agent_accesses"`
	ResolverAccesses      int      `json:"resolver_accesses"`
	ConfigurationAccesses int      `json:"configuration_accesses"`
	HookFilterCheckouts   int      `json:"hook_filter_checkout_accesses"`
	UnexpectedReads       int      `json:"unexpected_reads"`
	UnexpectedReadClasses []string `json:"unexpected_read_classes,omitempty"`
	UnknownEvents         int      `json:"unknown_events"`
	UnknownEventClasses   []string `json:"unknown_event_classes,omitempty"`
	TraceFiles            int      `json:"trace_files"`
	TraceComplete         bool     `json:"trace_complete"`
	SupervisorInvocations int      `json:"supervisor_invocations"`
	LifecycleComplete     bool     `json:"lifecycle_complete"`
	DeathGuardArmed       bool     `json:"death_guard_armed"`
	IOUringDenied         bool     `json:"io_uring_denied"`
	CgroupKillGuard       bool     `json:"cgroup_kill_guard"`
	DescendantsReaped     int      `json:"descendants_reaped"`
	Orphans               int      `json:"orphans"`
	Survivors             int      `json:"survivors"`
	MarkerSpills          int      `json:"marker_spills"`
}

type WriteObservation struct {
	ObservedPaths []string `json:"observed_paths"`
	OutsideWrites []string `json:"outside_writes"`
	AllowlistRoot string   `json:"allowlist_root"`
}

type ResourceObservation struct {
	CgroupInvocations int    `json:"cgroup_invocations"`
	CgroupComplete    bool   `json:"cgroup_complete"`
	MemoryPeakBytes   uint64 `json:"memory_peak_bytes"`
	ProcessPeak       uint64 `json:"process_peak"`
	CPUUsec           uint64 `json:"cpu_usec"`
	MemoryOOM         uint64 `json:"memory_oom"`
	OOMKill           uint64 `json:"oom_kill"`
	TransferBytes     int64  `json:"transfer_bytes"`
	DiskBytes         int64  `json:"disk_bytes"`
	ObjectCount       int    `json:"object_count"`
	ExpandedBytes     int64  `json:"expanded_bytes"`
	TreeEntries       int    `json:"tree_entries"`
	TreeDepth         int    `json:"tree_depth"`
}

type TreeObservation struct {
	Commit             string              `json:"commit"`
	RootTree           string              `json:"root_tree"`
	EmbeddedComponents []ComponentTree     `json:"embedded_components"`
	UnselectedGitlinks []GitlinkFact       `json:"unselected_gitlinks"`
	Materialized       []MaterializedEntry `json:"materialized"`
}

type ComponentTree struct {
	LogicalID string `json:"logical_id"`
	Path      string `json:"path"`
	Tree      string `json:"tree"`
}

type GitlinkFact struct {
	Path       string `json:"path"`
	Commit     string `json:"commit"`
	LocatorRef string `json:"locator_ref"`
	Kind       string `json:"kind"`
}

type MaterializedEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Size int64  `json:"size"`
}

type CleanupObservation struct {
	OwnedRootRemoved       bool `json:"owned_root_removed"`
	OutsideSentinelIntact  bool `json:"outside_sentinel_intact"`
	RedactedArtifactCopied bool `json:"redacted_artifact_copied"`
}
