package cmd

import (
	"testing"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/document"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

// minimalSchema returns a bare Schema sufficient to construct a Document.
func minimalSchema() *schema.Schema {
	return &schema.Schema{
		Entity:     "test",
		DocsDir:    "docs",
		RecordsDir: "records",
		Filename:   "{id}.md",
		IDField:    "id",
		Fields:     schema.FieldMap{},
	}
}

func newTestDoc() *document.Document {
	return document.New(minimalSchema(), "/tmp/testdb")
}

// ---------------------------------------------------------------------------
// parseFieldValue
// ---------------------------------------------------------------------------

func TestParseFieldValue(t *testing.T) {
	tests := []struct {
		input string
		check func(t *testing.T, got any)
	}{
		{
			"hello",
			func(t *testing.T, got any) {
				if s, ok := got.(string); !ok || s != "hello" {
					t.Errorf("want string \"hello\", got %T(%v)", got, got)
				}
			},
		},
		{
			"42",
			func(t *testing.T, got any) {
				if i, ok := got.(int); !ok || i != 42 {
					t.Errorf("want int(42), got %T(%v)", got, got)
				}
			},
		},
		{
			"3.14",
			func(t *testing.T, got any) {
				if f, ok := got.(float64); !ok || f != 3.14 {
					t.Errorf("want float64(3.14), got %T(%v)", got, got)
				}
			},
		},
		{
			"true",
			func(t *testing.T, got any) {
				if b, ok := got.(bool); !ok || !b {
					t.Errorf("want bool(true), got %T(%v)", got, got)
				}
			},
		},
		{
			"false",
			func(t *testing.T, got any) {
				if b, ok := got.(bool); !ok || b {
					t.Errorf("want bool(false), got %T(%v)", got, got)
				}
			},
		},
		{
			"[1,2,3]",
			func(t *testing.T, got any) {
				sl, ok := got.([]any)
				if !ok {
					t.Fatalf("want []any, got %T", got)
				}
				if len(sl) != 3 {
					t.Fatalf("want 3 elements, got %d", len(sl))
				}
				// JSON unmarshal yields float64; accept float64 or int
				for idx, item := range sl {
					expected := float64(idx + 1)
					switch v := item.(type) {
					case float64:
						if v != expected {
							t.Errorf("element %d: want %v, got %v", idx, expected, v)
						}
					case int:
						if float64(v) != expected {
							t.Errorf("element %d: want %v, got %v", idx, expected, v)
						}
					default:
						t.Errorf("element %d: unexpected type %T", idx, item)
					}
				}
			},
		},
		{
			`{"a":1}`,
			func(t *testing.T, got any) {
				m, ok := got.(map[string]any)
				if !ok {
					t.Fatalf("want map[string]any, got %T", got)
				}
				v, exists := m["a"]
				if !exists {
					t.Fatal("key \"a\" missing")
				}
				if f, ok := v.(float64); !ok || f != 1 {
					t.Errorf("want float64(1), got %T(%v)", v, v)
				}
			},
		},
		{
			"[invalid",
			func(t *testing.T, got any) {
				if s, ok := got.(string); !ok || s != "[invalid" {
					t.Errorf("want string \"[invalid\" fallback, got %T(%v)", got, got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tt.check(t, parseFieldValue(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// parseKVPairs
// ---------------------------------------------------------------------------

func TestParseKVPairs(t *testing.T) {
	t.Run("typed values", func(t *testing.T) {
		m := parseKVPairs([]string{"x=1", "y=hello", "z=true"})
		if i, ok := m["x"].(int); !ok || i != 1 {
			t.Errorf("x: want int(1), got %T(%v)", m["x"], m["x"])
		}
		if s, ok := m["y"].(string); !ok || s != "hello" {
			t.Errorf("y: want string \"hello\", got %T(%v)", m["y"], m["y"])
		}
		if b, ok := m["z"].(bool); !ok || !b {
			t.Errorf("z: want bool(true), got %T(%v)", m["z"], m["z"])
		}
	})

	t.Run("bare key (no =)", func(t *testing.T) {
		m := parseKVPairs([]string{"bare"})
		// Current implementation: result[parts[0]] = fmt.Sprintf("%v", true) → string "true"
		if s, ok := m["bare"].(string); !ok || s != "true" {
			t.Errorf("bare: want string \"true\", got %T(%v)", m["bare"], m["bare"])
		}
	})
}

// ---------------------------------------------------------------------------
// applyFieldUpdate
// ---------------------------------------------------------------------------

func TestApplyFieldUpdate(t *testing.T) {
	t.Run("set x=42", func(t *testing.T) {
		doc := newTestDoc()
		if err := applyFieldUpdate(doc, "x=42"); err != nil {
			t.Fatal(err)
		}
		if i, ok := doc.Data["x"].(int); !ok || i != 42 {
			t.Errorf("want int(42), got %T(%v)", doc.Data["x"], doc.Data["x"])
		}
	})

	t.Run("append to existing list", func(t *testing.T) {
		doc := newTestDoc()
		doc.Data["x"] = []any{2}
		if err := applyFieldUpdate(doc, "x+=1"); err != nil {
			t.Fatal(err)
		}
		sl, ok := doc.Data["x"].([]any)
		if !ok || len(sl) != 2 {
			t.Fatalf("want []any of len 2, got %T(%v)", doc.Data["x"], doc.Data["x"])
		}
		if i, ok := sl[1].(int); !ok || i != 1 {
			t.Errorf("appended element: want int(1), got %T(%v)", sl[1], sl[1])
		}
	})

	t.Run("append to nil/missing creates list", func(t *testing.T) {
		doc := newTestDoc()
		if err := applyFieldUpdate(doc, "x+=1"); err != nil {
			t.Fatal(err)
		}
		sl, ok := doc.Data["x"].([]any)
		if !ok || len(sl) != 1 {
			t.Fatalf("want []any{1}, got %T(%v)", doc.Data["x"], doc.Data["x"])
		}
	})

	t.Run("append to non-list errors", func(t *testing.T) {
		doc := newTestDoc()
		doc.Data["x"] = "oops"
		err := applyFieldUpdate(doc, "x+=1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !containsStr(err.Error(), "cannot append") {
			t.Errorf("error should contain \"cannot append\", got: %s", err.Error())
		}
	})

	t.Run("remove from list", func(t *testing.T) {
		doc := newTestDoc()
		doc.Data["x"] = []any{1, 2, 3}
		if err := applyFieldUpdate(doc, "x-=2"); err != nil {
			t.Fatal(err)
		}
		sl, ok := doc.Data["x"].([]any)
		if !ok {
			t.Fatalf("want []any, got %T", doc.Data["x"])
		}
		if len(sl) != 2 {
			t.Fatalf("want 2 elements after remove, got %d: %v", len(sl), sl)
		}
		for _, item := range sl {
			if s := formatAny(item); s == "2" {
				t.Errorf("element 2 should have been removed, still present: %v", sl)
			}
		}
	})

	t.Run("remove from non-list errors", func(t *testing.T) {
		doc := newTestDoc()
		doc.Data["x"] = "scalar"
		err := applyFieldUpdate(doc, "x-=2")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !containsStr(err.Error(), "cannot remove") {
			t.Errorf("error should contain \"cannot remove\", got: %s", err.Error())
		}
	})

	t.Run("delete with ~=", func(t *testing.T) {
		doc := newTestDoc()
		doc.Data["x"] = "something"
		if err := applyFieldUpdate(doc, "x~=anything"); err != nil {
			t.Fatal(err)
		}
		if _, exists := doc.Data["x"]; exists {
			t.Error("key x should have been deleted")
		}
	})

	t.Run("malformed (no =) errors", func(t *testing.T) {
		doc := newTestDoc()
		err := applyFieldUpdate(doc, "malformed")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !containsStr(err.Error(), "invalid --field") {
			t.Errorf("error should contain \"invalid --field\", got: %s", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// appendToList / removeFromList (direct)
// ---------------------------------------------------------------------------

func TestAppendToList(t *testing.T) {
	t.Run("append to []any", func(t *testing.T) {
		doc := newTestDoc()
		doc.Data["tags"] = []any{"a"}
		if err := appendToList(doc, "tags", "b"); err != nil {
			t.Fatal(err)
		}
		sl := doc.Data["tags"].([]any)
		if len(sl) != 2 || sl[1] != "b" {
			t.Errorf("unexpected result: %v", sl)
		}
	})

	t.Run("append to nil creates list", func(t *testing.T) {
		doc := newTestDoc()
		if err := appendToList(doc, "tags", "x"); err != nil {
			t.Fatal(err)
		}
		sl := doc.Data["tags"].([]any)
		if len(sl) != 1 || sl[0] != "x" {
			t.Errorf("unexpected result: %v", sl)
		}
	})

	t.Run("append to non-list errors", func(t *testing.T) {
		doc := newTestDoc()
		doc.Data["tags"] = "not-a-list"
		err := appendToList(doc, "tags", "x")
		if err == nil || !containsStr(err.Error(), "cannot append") {
			t.Errorf("expected cannot-append error, got: %v", err)
		}
	})
}

func TestRemoveFromList(t *testing.T) {
	t.Run("removes matching element", func(t *testing.T) {
		doc := newTestDoc()
		doc.Data["tags"] = []any{"a", "b", "c"}
		if err := removeFromList(doc, "tags", "b"); err != nil {
			t.Fatal(err)
		}
		sl := doc.Data["tags"].([]any)
		if len(sl) != 2 {
			t.Errorf("expected 2 elements, got %d: %v", len(sl), sl)
		}
	})

	t.Run("error on non-list", func(t *testing.T) {
		doc := newTestDoc()
		doc.Data["tags"] = 42
		err := removeFromList(doc, "tags", "x")
		if err == nil || !containsStr(err.Error(), "cannot remove") {
			t.Errorf("expected cannot-remove error, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

func formatAny(v any) string {
	return func() string {
		switch x := v.(type) {
		case string:
			return x
		case int:
			if x == 2 {
				return "2"
			}
		}
		return ""
	}()
}
