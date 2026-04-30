package runtime

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefault_FieldsAreSafe(t *testing.T) {
	r := Default()
	assert.NotNil(t, r.Logger)
	assert.NotNil(t, r.Clock)
	assert.Nil(t, r.KeyLoader)
	assert.Greater(t, r.WalkWorkers, 0)
	t1 := r.Clock()
	time.Sleep(time.Millisecond)
	t2 := r.Clock()
	assert.True(t, t2.After(t1) || t2.Equal(t1))
}

func TestWithDefaults_BackfillsNilFields(t *testing.T) {
	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := Runtime{Logger: custom}.WithDefaults()
	assert.Equal(t, custom, r.Logger)
	assert.NotNil(t, r.Clock)
	assert.Greater(t, r.WalkWorkers, 0)
}
