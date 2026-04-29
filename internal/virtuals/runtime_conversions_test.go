package virtuals

import (
	"testing"

	"go.starlark.net/starlark"
)

// ---------------------------------------------------------------------------
// goToStarlark
// ---------------------------------------------------------------------------

func TestGoToStarlark(t *testing.T) {
	t.Run("nil → None", func(t *testing.T) {
		v, err := goToStarlark(nil)
		if err != nil {
			t.Fatal(err)
		}
		if v != starlark.None {
			t.Errorf("want starlark.None, got %v", v)
		}
	})

	t.Run("string", func(t *testing.T) {
		v, err := goToStarlark("hello")
		if err != nil {
			t.Fatal(err)
		}
		s, ok := v.(starlark.String)
		if !ok || string(s) != "hello" {
			t.Errorf("want starlark.String(\"hello\"), got %T(%v)", v, v)
		}
	})

	t.Run("bool true", func(t *testing.T) {
		v, err := goToStarlark(true)
		if err != nil {
			t.Fatal(err)
		}
		b, ok := v.(starlark.Bool)
		if !ok || !bool(b) {
			t.Errorf("want starlark.Bool(true), got %T(%v)", v, v)
		}
	})

	t.Run("bool false", func(t *testing.T) {
		v, err := goToStarlark(false)
		if err != nil {
			t.Fatal(err)
		}
		b, ok := v.(starlark.Bool)
		if !ok || bool(b) {
			t.Errorf("want starlark.Bool(false), got %T(%v)", v, v)
		}
	})

	t.Run("int(42)", func(t *testing.T) {
		v, err := goToStarlark(int(42))
		if err != nil {
			t.Fatal(err)
		}
		i, ok := v.(starlark.Int)
		if !ok {
			t.Fatalf("want starlark.Int, got %T", v)
		}
		n, ok2 := i.Int64()
		if !ok2 || n != 42 {
			t.Errorf("want 42, got %v", i)
		}
	})

	t.Run("int64(42)", func(t *testing.T) {
		v, err := goToStarlark(int64(42))
		if err != nil {
			t.Fatal(err)
		}
		i, ok := v.(starlark.Int)
		if !ok {
			t.Fatalf("want starlark.Int, got %T", v)
		}
		n, ok2 := i.Int64()
		if !ok2 || n != 42 {
			t.Errorf("want 42, got %v", i)
		}
	})

	t.Run("float64(3.14)", func(t *testing.T) {
		v, err := goToStarlark(float64(3.14))
		if err != nil {
			t.Fatal(err)
		}
		f, ok := v.(starlark.Float)
		if !ok || float64(f) != 3.14 {
			t.Errorf("want starlark.Float(3.14), got %T(%v)", v, v)
		}
	})

	t.Run("[]any list", func(t *testing.T) {
		v, err := goToStarlark([]any{1, "two", true})
		if err != nil {
			t.Fatal(err)
		}
		l, ok := v.(*starlark.List)
		if !ok {
			t.Fatalf("want *starlark.List, got %T", v)
		}
		if l.Len() != 3 {
			t.Errorf("want 3 elements, got %d", l.Len())
		}
	})

	t.Run("[]string list", func(t *testing.T) {
		v, err := goToStarlark([]string{"a", "b"})
		if err != nil {
			t.Fatal(err)
		}
		l, ok := v.(*starlark.List)
		if !ok {
			t.Fatalf("want *starlark.List, got %T", v)
		}
		if l.Len() != 2 {
			t.Errorf("want 2 elements, got %d", l.Len())
		}
	})

	t.Run("map[string]any → Dict", func(t *testing.T) {
		v, err := goToStarlark(map[string]any{"k": "v"})
		if err != nil {
			t.Fatal(err)
		}
		d, ok := v.(*starlark.Dict)
		if !ok {
			t.Fatalf("want *starlark.Dict, got %T", v)
		}
		val, found, err2 := d.Get(starlark.String("k"))
		if err2 != nil || !found {
			t.Fatal("key \"k\" not found in dict")
		}
		if s, ok := val.(starlark.String); !ok || string(s) != "v" {
			t.Errorf("want \"v\", got %v", val)
		}
	})

	t.Run("unsupported type → string fallback", func(t *testing.T) {
		v, err := goToStarlark(struct{}{})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := v.(starlark.String); !ok {
			t.Errorf("want starlark.String fallback, got %T", v)
		}
	})
}

// ---------------------------------------------------------------------------
// starlarkToGo
// ---------------------------------------------------------------------------

func TestStarlarkToGo(t *testing.T) {
	t.Run("None → nil", func(t *testing.T) {
		v, err := starlarkToGo(starlark.None)
		if err != nil || v != nil {
			t.Errorf("want nil, got %v (err=%v)", v, err)
		}
	})

	t.Run("String", func(t *testing.T) {
		v, err := starlarkToGo(starlark.String("hello"))
		if err != nil {
			t.Fatal(err)
		}
		if s, ok := v.(string); !ok || s != "hello" {
			t.Errorf("want \"hello\", got %T(%v)", v, v)
		}
	})

	t.Run("Bool true", func(t *testing.T) {
		v, err := starlarkToGo(starlark.Bool(true))
		if err != nil {
			t.Fatal(err)
		}
		if b, ok := v.(bool); !ok || !b {
			t.Errorf("want bool(true), got %T(%v)", v, v)
		}
	})

	t.Run("Int via MakeInt64", func(t *testing.T) {
		v, err := starlarkToGo(starlark.MakeInt64(42))
		if err != nil {
			t.Fatal(err)
		}
		if i, ok := v.(int); !ok || i != 42 {
			t.Errorf("want int(42), got %T(%v)", v, v)
		}
	})

	t.Run("Float", func(t *testing.T) {
		v, err := starlarkToGo(starlark.Float(3.14))
		if err != nil {
			t.Fatal(err)
		}
		if f, ok := v.(float64); !ok || f != 3.14 {
			t.Errorf("want float64(3.14), got %T(%v)", v, v)
		}
	})

	t.Run("List of strings", func(t *testing.T) {
		l := starlark.NewList([]starlark.Value{starlark.String("a"), starlark.String("b")})
		v, err := starlarkToGo(l)
		if err != nil {
			t.Fatal(err)
		}
		sl, ok := v.([]any)
		if !ok || len(sl) != 2 {
			t.Fatalf("want []any{\"a\",\"b\"}, got %T(%v)", v, v)
		}
		if sl[0] != "a" || sl[1] != "b" {
			t.Errorf("unexpected values: %v", sl)
		}
	})

	t.Run("Dict", func(t *testing.T) {
		d := starlark.NewDict(1)
		_ = d.SetKey(starlark.String("k"), starlark.String("v"))
		v, err := starlarkToGo(d)
		if err != nil {
			t.Fatal(err)
		}
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("want map[string]any, got %T", v)
		}
		if m["k"] != "v" {
			t.Errorf("want m[\"k\"]==\"v\", got %v", m["k"])
		}
	})

	t.Run("Tuple", func(t *testing.T) {
		tup := starlark.Tuple{starlark.String("x"), starlark.MakeInt(1)}
		v, err := starlarkToGo(tup)
		if err != nil {
			t.Fatal(err)
		}
		sl, ok := v.([]any)
		if !ok || len(sl) != 2 {
			t.Fatalf("want []any of len 2, got %T(%v)", v, v)
		}
		if sl[0] != "x" {
			t.Errorf("want \"x\", got %v", sl[0])
		}
	})

	t.Run("default fallback → string", func(t *testing.T) {
		// starlark.Float implements starlark.Value but is handled; use a custom Value
		// that hits the default branch. The easiest is to use starlark.Bytes which
		// is a valid starlark.Value not covered by the switch.
		byt := starlark.Bytes("raw")
		v, err := starlarkToGo(byt)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := v.(string); !ok {
			t.Errorf("want string fallback, got %T(%v)", v, v)
		}
	})
}

// ---------------------------------------------------------------------------
// Round-trip tests
// ---------------------------------------------------------------------------

func TestRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		val  any
	}{
		{"string", "hello"},
		{"int", 42},
		{"list", []any{"x", "y"}},
		{"map", map[string]any{"key": "value"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sv, err := goToStarlark(tc.val)
			if err != nil {
				t.Fatalf("goToStarlark: %v", err)
			}
			got, err := starlarkToGo(sv)
			if err != nil {
				t.Fatalf("starlarkToGo: %v", err)
			}
			// For maps and slices, we can't use == directly; check type at least.
			switch expected := tc.val.(type) {
			case string:
				if got != expected {
					t.Errorf("round-trip: want %v, got %v", expected, got)
				}
			case int:
				if got != expected {
					t.Errorf("round-trip: want %v, got %v", expected, got)
				}
			case []any:
				sl, ok := got.([]any)
				if !ok || len(sl) != len(expected) {
					t.Errorf("round-trip slice: want %v, got %v", expected, got)
				}
			case map[string]any:
				m, ok := got.(map[string]any)
				if !ok {
					t.Errorf("round-trip map: want map, got %T", got)
				}
				for k, ev := range expected {
					if m[k] != ev {
						t.Errorf("round-trip map[%q]: want %v, got %v", k, ev, m[k])
					}
				}
			}
		})
	}
}
