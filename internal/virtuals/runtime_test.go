package virtuals

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntime_TitleVirtual(t *testing.T) {
	rt := NewRuntime()
	err := rt.Compile("title", `
def compute(content, fields):
    for line in content.splitlines():
        if line.startswith("# "):
            return line.removeprefix("# ").strip()
    return fields["id"]
`, "string")
	require.NoError(t, err)

	result, err := rt.Evaluate("title", "# Hello World\n\nSome content.", map[string]any{"id": "fallback"})
	require.NoError(t, err)
	assert.Equal(t, "Hello World", result)
}

func TestRuntime_TitleVirtual_Fallback(t *testing.T) {
	rt := NewRuntime()
	err := rt.Compile("title", `
def compute(content, fields):
    for line in content.splitlines():
        if line.startswith("# "):
            return line.removeprefix("# ").strip()
    return fields["id"]
`, "string")
	require.NoError(t, err)

	result, err := rt.Evaluate("title", "No heading here.", map[string]any{"id": "my-note"})
	require.NoError(t, err)
	assert.Equal(t, "my-note", result)
}

func TestRuntime_WordCount(t *testing.T) {
	rt := NewRuntime()
	err := rt.Compile("word_count", `
def compute(content, fields):
    return len(content.split())
`, "int")
	require.NoError(t, err)

	result, err := rt.Evaluate("word_count", "one two three four five", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 5, result)
}

func TestRuntime_RegexFindall(t *testing.T) {
	rt := NewRuntime()
	err := rt.Compile("ticket_refs", `
def compute(content, fields):
    return re.findall("[A-Z]+-[0-9]+", content)
`, "list[string]")
	require.NoError(t, err)

	result, err := rt.Evaluate("ticket_refs",
		"Fixed PROJ-123 and also TEAM-456 were involved.",
		map[string]any{})
	require.NoError(t, err)

	refs, ok := result.([]any)
	require.True(t, ok)
	assert.Len(t, refs, 2)
	assert.Equal(t, "PROJ-123", refs[0])
	assert.Equal(t, "TEAM-456", refs[1])
}

func TestRuntime_Deterministic(t *testing.T) {
	rt := NewRuntime()
	err := rt.Compile("title", `
def compute(content, fields):
    return content.split("\n")[0].strip()
`, "string")
	require.NoError(t, err)

	r1, _ := rt.Evaluate("title", "Hello\nWorld", map[string]any{})
	r2, _ := rt.Evaluate("title", "Hello\nWorld", map[string]any{})
	assert.Equal(t, r1, r2, "same input must produce same output")
}

func TestRuntime_EvaluateAll(t *testing.T) {
	rt := NewRuntime()
	require.NoError(t, rt.Compile("title", `
def compute(content, fields):
    return "Title"
`, "string"))
	require.NoError(t, rt.Compile("count", `
def compute(content, fields):
    return len(content)
`, "int"))

	results, err := rt.EvaluateAll("hello", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "Title", results["title"])
	assert.Equal(t, 5, results["count"])
}

func TestRuntime_CompileError(t *testing.T) {
	rt := NewRuntime()
	err := rt.Compile("bad", `
def compute(content fields):  # missing comma
    return content
`, "string")
	assert.Error(t, err)
}

func TestRuntime_MissingComputeFunction(t *testing.T) {
	rt := NewRuntime()
	err := rt.Compile("no_compute", `
x = 42
`, "string")
	// Now fails at compile time since we Init eagerly to extract compute
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing compute")
}
