package sync

import (
	"strings"
	"testing"
)

func TestValidateConfigOK(t *testing.T) {
	c := IntegrationConfig{
		Integration: "confluence",
		AppliesTo: map[string]EntityBinding{
			"notes": {
				TargetRef: "sync.confluence.pageId",
				Payload: map[string]PayloadField{
					"title": {From: "frontmatter.title"},
					"body":  {From: "rendered_markdown"},
				},
			},
		},
	}
	known := map[string]bool{"notes": true}
	if err := ValidateConfig(c, known); err != nil {
		t.Fatalf("ValidateConfig OK case = %v", err)
	}
}

func TestValidateConfigUnknownEntity(t *testing.T) {
	c := IntegrationConfig{
		Integration: "confluence",
		AppliesTo:   map[string]EntityBinding{"ghosts": {TargetRef: "x.y", Payload: map[string]PayloadField{"t": {From: "frontmatter.title"}}}},
	}
	err := ValidateConfig(c, map[string]bool{"notes": true})
	if err == nil || !strings.Contains(err.Error(), "ghosts") {
		t.Fatalf("want error mentioning ghosts, got %v", err)
	}
}

func TestValidateConfigEmptyTargetRef(t *testing.T) {
	c := IntegrationConfig{
		Integration: "confluence",
		AppliesTo:   map[string]EntityBinding{"notes": {TargetRef: "", Payload: map[string]PayloadField{"t": {From: "frontmatter.title"}}}},
	}
	err := ValidateConfig(c, map[string]bool{"notes": true})
	if err == nil || !strings.Contains(err.Error(), "target_ref") {
		t.Fatalf("want target_ref error, got %v", err)
	}
}

func TestValidateConfigUnknownPayloadSymbol(t *testing.T) {
	c := IntegrationConfig{
		Integration: "confluence",
		AppliesTo: map[string]EntityBinding{
			"notes": {TargetRef: "sync.x.id", Payload: map[string]PayloadField{
				"title": {From: "wat.does.this.mean"},
			}},
		},
	}
	err := ValidateConfig(c, map[string]bool{"notes": true})
	if err == nil || !strings.Contains(err.Error(), "unknown payload source") {
		t.Fatalf("want unknown-payload-source error, got %v", err)
	}
}

func TestValidateConfigConstOK(t *testing.T) {
	c := IntegrationConfig{
		Integration: "confluence",
		AppliesTo: map[string]EntityBinding{
			"runbooks": {TargetRef: "sync.x.id", Payload: map[string]PayloadField{
				"labels": {Const: []interface{}{"runbook"}},
			}},
		},
	}
	if err := ValidateConfig(c, map[string]bool{"runbooks": true}); err != nil {
		t.Fatalf("const-payload case = %v", err)
	}
}

func TestValidateConfigPayloadFieldNeitherFromNorConst(t *testing.T) {
	c := IntegrationConfig{
		Integration: "confluence",
		AppliesTo: map[string]EntityBinding{
			"notes": {TargetRef: "sync.x.id", Payload: map[string]PayloadField{
				"title": {},
			}},
		},
	}
	err := ValidateConfig(c, map[string]bool{"notes": true})
	if err == nil || !strings.Contains(err.Error(), "must have 'from' or 'const'") {
		t.Fatalf("want neither-from-nor-const error, got %v", err)
	}
}
