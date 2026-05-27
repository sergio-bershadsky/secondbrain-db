package sync

import "testing"

func TestResolveBackrefDotted(t *testing.T) {
	fm := map[string]interface{}{
		"sync": map[string]interface{}{
			"confluence": map[string]interface{}{"pageId": "12345"},
			"jira":       map[string]interface{}{"issueKey": "ENG-42"},
		},
	}
	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{"sync.confluence.pageId", "12345", true},
		{"sync.jira.issueKey", "ENG-42", true},
		{"sync.confluence.missing", "", false},
		{"missing.path", "", false},
		{"sync", "", false}, // intermediate node, not a leaf string
	}
	for _, c := range cases {
		got, ok := ResolveBackref(fm, c.path)
		if got != c.want || ok != c.ok {
			t.Errorf("ResolveBackref(%q) = (%q, %v), want (%q, %v)",
				c.path, got, ok, c.want, c.ok)
		}
	}
}

func TestResolveBackrefIntCoerced(t *testing.T) {
	fm := map[string]interface{}{"sync": map[string]interface{}{"jira": map[string]interface{}{"issueKey": 42}}}
	got, ok := ResolveBackref(fm, "sync.jira.issueKey")
	if !ok || got != "42" {
		t.Errorf("ResolveBackref int = (%q, %v), want (42, true)", got, ok)
	}
}
