package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanonicalString_Scalars(t *testing.T) {
	assert.Equal(t, `"hello"`, CanonicalString("hello"))
	assert.Equal(t, "42", CanonicalString(42))
	assert.Equal(t, "42", CanonicalString(int64(42)))
	assert.Equal(t, "42", CanonicalString(float64(42)))
	assert.Equal(t, "3.14", CanonicalString(3.14))
	assert.Equal(t, "true", CanonicalString(true))
	assert.Equal(t, "false", CanonicalString(false))
	assert.Equal(t, "null", CanonicalString(nil))
}

func TestCanonicalString_Map_SortedKeys(t *testing.T) {
	m := map[string]any{"b": 2, "a": 1, "c": 3}
	result := CanonicalString(m)
	assert.Equal(t, `{"a":1,"b":2,"c":3}`, result)
}

func TestCanonicalString_NestedMap(t *testing.T) {
	m := map[string]any{
		"z": map[string]any{"b": "x", "a": "y"},
		"a": 1,
	}
	result := CanonicalString(m)
	assert.Equal(t, `{"a":1,"z":{"a":"y","b":"x"}}`, result)
}

func TestCanonicalString_Slice(t *testing.T) {
	s := []any{"b", "a", 1}
	result := CanonicalString(s)
	assert.Equal(t, `["b","a",1]`, result)
}

func TestCanonicalHash_Deterministic(t *testing.T) {
	m1 := map[string]any{"b": 2, "a": 1}
	m2 := map[string]any{"a": 1, "b": 2}
	assert.Equal(t, CanonicalHash(m1), CanonicalHash(m2),
		"key order should not affect hash")
}

func TestCanonicalBodyHash_NormalizesTrailingNewline(t *testing.T) {
	h1 := CanonicalBodyHash("hello\n")
	h2 := CanonicalBodyHash("hello\n\n")
	h3 := CanonicalBodyHash("hello")
	assert.Equal(t, h1, h2, "trailing newlines should be normalized")
	assert.Equal(t, h1, h3, "missing trailing newline should be added")
}

func TestCanonicalString_DistinguishesTypes(t *testing.T) {
	// "null" vs null
	assert.NotEqual(t, CanonicalString("null"), CanonicalString(nil))
	// "123" vs 123
	assert.NotEqual(t, CanonicalString("123"), CanonicalString(123))
	// "true" vs true
	assert.NotEqual(t, CanonicalString("true"), CanonicalString(true))
}
