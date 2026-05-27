package sync

import (
	"fmt"
	"os"
	"strings"
)

// LinkStatus describes whether a doc has a target ID for an integration.
type LinkStatus string

const (
	StatusLinked   LinkStatus = "linked"
	StatusUnlinked LinkStatus = "unlinked"
)

// ResolvedDocIntegration is the full picture for one (doc, integration) pair,
// suitable for handing to an integration runtime before a push.
type ResolvedDocIntegration struct {
	Integration    string                 `json:"integration"`
	Entity         string                 `json:"entity"`
	DocPath        string                 `json:"doc_path"`
	TargetRef      string                 `json:"target_ref"`
	TargetID       string                 `json:"target_id"`
	Status         LinkStatus             `json:"status"`
	Required       bool                   `json:"required"`
	Payload        map[string]interface{} `json:"payload"`
	DriftResult    DriftResult            `json:"-"`
	DriftString    string                 `json:"drift_result"`
	CurrentDocHash string                 `json:"current_doc_hash"`
	State          SidecarSection         `json:"state"`
}

// ResolveDocIntegration loads the doc, parses frontmatter, resolves the
// back-ref, computes the payload from the mapping, and computes local drift.
// No network call — observedRemoteRev is left empty.
func ResolveDocIntegration(docPath, entity string, cfg IntegrationConfig) (*ResolvedDocIntegration, error) {
	binding, ok := cfg.AppliesTo[entity]
	if !ok {
		return nil, fmt.Errorf("integration %q has no applies_to entry for entity %q", cfg.Integration, entity)
	}
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		return nil, fmt.Errorf("read doc %s: %w", docPath, err)
	}
	fm, err := ParseFrontmatter(docBytes)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter %s: %w", docPath, err)
	}
	targetID, linked := ResolveBackref(fm, binding.TargetRef)
	payload, err := assemblePayload(binding.Payload, fm, docBytes)
	if err != nil {
		return nil, fmt.Errorf("assemble payload: %w", err)
	}
	docHash, err := ComputeDocHash(docBytes)
	if err != nil {
		return nil, fmt.Errorf("hash doc: %w", err)
	}
	sc, err := ReadSidecar(docPath)
	if err != nil {
		return nil, err
	}
	section := sc[cfg.Integration]
	drift := ComputeDrift(section, docHash, "")

	status := StatusUnlinked
	if linked {
		status = StatusLinked
	}

	return &ResolvedDocIntegration{
		Integration:    cfg.Integration,
		Entity:         entity,
		DocPath:        docPath,
		TargetRef:      binding.TargetRef,
		TargetID:       targetID,
		Status:         status,
		Required:       binding.Required,
		Payload:        payload,
		DriftResult:    drift,
		DriftString:    drift.String(),
		CurrentDocHash: docHash,
		State:          section,
	}, nil
}

func assemblePayload(mapping map[string]PayloadField, fm map[string]interface{}, doc []byte) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for field, pf := range mapping {
		switch {
		case pf.Const != nil:
			out[field] = pf.Const
		case pf.From == "rendered_markdown" || pf.From == "body":
			body, err := RenderedMarkdown(doc)
			if err != nil {
				return nil, err
			}
			out[field] = body
		case strings.HasPrefix(pf.From, "frontmatter."):
			path := strings.TrimPrefix(pf.From, "frontmatter.")
			out[field] = walkPath(fm, path)
		default:
			return nil, fmt.Errorf("payload.%s: unknown source %q", field, pf.From)
		}
	}
	return out, nil
}

func walkPath(fm map[string]interface{}, dotted string) interface{} {
	parts := strings.Split(dotted, ".")
	var cur interface{} = fm
	for _, p := range parts {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}
