package schema

import (
	"testing"
)

// ---------------------------------------------------------------------------
// asInt
// ---------------------------------------------------------------------------

func TestAsInt(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantVal int64
		wantOK  bool
	}{
		{"int(42)", int(42), 42, true},
		{"int64(42)", int64(42), 42, true},
		{"float64 integral", float64(42.0), 42, true},
		{"float64 non-integral", float64(42.5), 0, false},
		{"string", "42", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asInt(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok: want %v, got %v", tt.wantOK, ok)
			}
			if ok && got != tt.wantVal {
				t.Errorf("val: want %d, got %d", tt.wantVal, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// asFloat
// ---------------------------------------------------------------------------

func TestAsFloat(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantVal float64
		wantOK  bool
	}{
		{"float64(3.14)", float64(3.14), 3.14, true},
		{"int(3)", int(3), 3.0, true},
		{"int64(3)", int64(3), 3.0, true},
		{"string", "3.14", 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asFloat(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok: want %v, got %v", tt.wantOK, ok)
			}
			if ok && got != tt.wantVal {
				t.Errorf("val: want %v, got %v", tt.wantVal, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// asSlice
// ---------------------------------------------------------------------------

func TestAsSlice(t *testing.T) {
	t.Run("[]any passthrough", func(t *testing.T) {
		input := []any{1, 2}
		got, ok := asSlice(input)
		if !ok {
			t.Fatal("want ok=true")
		}
		if len(got) != 2 {
			t.Errorf("want 2 elements, got %d", len(got))
		}
	})

	t.Run("[]string converts to []any", func(t *testing.T) {
		input := []string{"a", "b"}
		got, ok := asSlice(input)
		if !ok {
			t.Fatal("want ok=true")
		}
		if len(got) != 2 {
			t.Fatalf("want 2 elements, got %d", len(got))
		}
		if got[0] != "a" || got[1] != "b" {
			t.Errorf("want [\"a\",\"b\"], got %v", got)
		}
	})

	t.Run("string rejected", func(t *testing.T) {
		got, ok := asSlice("abc")
		if ok || got != nil {
			t.Errorf("want (nil, false), got (%v, %v)", got, ok)
		}
	})

	t.Run("int rejected", func(t *testing.T) {
		got, ok := asSlice(42)
		if ok || got != nil {
			t.Errorf("want (nil, false), got (%v, %v)", got, ok)
		}
	})

	t.Run("nil rejected", func(t *testing.T) {
		got, ok := asSlice(nil)
		if ok || got != nil {
			t.Errorf("want (nil, false), got (%v, %v)", got, ok)
		}
	})
}

// ---------------------------------------------------------------------------
// asString
// ---------------------------------------------------------------------------

func TestAsString(t *testing.T) {
	// asString accepts string and fmt.Stringer; rejects others.
	t.Run("string accepted", func(t *testing.T) {
		got, ok := asString("hello")
		if !ok || got != "hello" {
			t.Errorf("want (\"hello\", true), got (%q, %v)", got, ok)
		}
	})

	t.Run("int rejected", func(t *testing.T) {
		_, ok := asString(42)
		if ok {
			t.Error("want ok=false for int")
		}
	})

	t.Run("bool rejected", func(t *testing.T) {
		_, ok := asString(true)
		if ok {
			t.Error("want ok=false for bool")
		}
	})

	t.Run("nil rejected", func(t *testing.T) {
		_, ok := asString(nil)
		if ok {
			t.Error("want ok=false for nil")
		}
	})

	t.Run("fmt.Stringer accepted", func(t *testing.T) {
		// stringerType is defined at file level (below) to satisfy fmt.Stringer.
		sv := stringerType("world")
		got, ok := asString(sv)
		if !ok || got != "world" {
			t.Errorf("want (\"world\", true), got (%q, %v)", got, ok)
		}
	})
}

// stringerType satisfies fmt.Stringer so we can exercise the Stringer branch of asString.
type stringerType string

func (s stringerType) String() string { return string(s) }
