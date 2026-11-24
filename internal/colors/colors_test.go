package colors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForColumnCycles(t *testing.T) {
	assert.Equal(t, "#7daea3", HexForColumn(0))
	assert.Equal(t, "#7daea3", HexForColumn(7))
	assert.Equal(t, "#d3869b", HexForColumn(2))
}

func TestForColumnCached(t *testing.T) {
	assert.Same(t, ForColumn(3), ForColumn(3))
	assert.Same(t, ForColumn(0), ForColumn(7))
	assert.NotSame(t, ForColumn(0), ForColumn(1))
}
