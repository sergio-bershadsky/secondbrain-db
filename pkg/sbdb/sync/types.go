package sync

import "fmt"

// DriftResult is the per-(doc, integration) state reported by `sbdb sync check`.
type DriftResult int

const (
	ResultUnknown DriftResult = iota
	ResultInSync
	ResultLocalDrift
	ResultRemoteDrift
	ResultBothDrift
	ResultNeverPublished
)

func (r DriftResult) String() string {
	switch r {
	case ResultInSync:
		return "in_sync"
	case ResultLocalDrift:
		return "local_drift"
	case ResultRemoteDrift:
		return "remote_drift"
	case ResultBothDrift:
		return "both_drift"
	case ResultNeverPublished:
		return "never_published"
	}
	return "unknown"
}

// ParseDriftResult is the inverse of String. Used when reading sidecars.
func ParseDriftResult(s string) (DriftResult, error) {
	switch s {
	case "in_sync":
		return ResultInSync, nil
	case "local_drift":
		return ResultLocalDrift, nil
	case "remote_drift":
		return ResultRemoteDrift, nil
	case "both_drift":
		return ResultBothDrift, nil
	case "never_published":
		return ResultNeverPublished, nil
	}
	return ResultUnknown, fmt.Errorf("unknown drift result %q", s)
}

// IntegrationConfig is one .sbdb/integrations/<name>.yaml file, parsed.
type IntegrationConfig struct {
	Integration string                   `yaml:"integration"`
	AppliesTo   map[string]EntityBinding `yaml:"applies_to"`
	// SourcePath is the disk path the config was loaded from. Not in YAML.
	SourcePath string `yaml:"-"`
}

// EntityBinding declares how a specific entity type publishes via this integration.
type EntityBinding struct {
	TargetRef string                  `yaml:"target_ref"`
	Required  bool                    `yaml:"required"`
	Payload   map[string]PayloadField `yaml:"payload"`
}

// PayloadField is one mapped field. Exactly one of From or Const is set.
type PayloadField struct {
	From  string      `yaml:"from,omitempty"`
	Const interface{} `yaml:"const,omitempty"`
}

// Sidecar is the on-disk per-doc state file `<doc>.integrations.yaml`.
// Top-level keys are integration names; each maps to a SidecarSection.
type Sidecar map[string]SidecarSection

// SidecarSection is the state for one (doc, integration) pair.
type SidecarSection struct {
	TargetID  string       `yaml:"target_id"`
	LastPush  *PushRecord  `yaml:"last_push"`
	LastCheck *CheckRecord `yaml:"last_check"`
	LastError *ErrorRecord `yaml:"last_error"`
}

// PushRecord is the durable record of the last successful push.
type PushRecord struct {
	DocHash        string `yaml:"doc_hash"`
	At             string `yaml:"at"`
	RemoteRevision string `yaml:"remote_revision"`
	Actor          string `yaml:"actor"`
}

// CheckRecord is the transient record of the last drift check. Overwritten
// on every check; not durable.
type CheckRecord struct {
	At                     string `yaml:"at"`
	Result                 string `yaml:"result"`
	RemoteRevisionObserved string `yaml:"remote_revision_observed,omitempty"`
	CurrentDocHash         string `yaml:"current_doc_hash,omitempty"`
}

// ErrorRecord is the transient record of the last failed attempt. Overwritten
// on the next attempt (success or failure).
type ErrorRecord struct {
	At               string `yaml:"at"`
	Stage            string `yaml:"stage"` // "push" or "check"
	Message          string `yaml:"message"`
	AttemptedDocHash string `yaml:"attempted_doc_hash,omitempty"`
}
