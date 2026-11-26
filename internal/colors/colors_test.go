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

func TestMix(t *testing.T) {
	assert.Equal(t, "#808080", Mix("#ffffff", "#000000", 0.5))
	assert.Equal(t, "#282828", Mix("#7daea3", "#282828", 0))
	assert.Equal(t, "#7daea3", Mix("#7daea3", "#282828", 1))
	assert.Equal(t, "#394341", Mix("#7daea3", Background, 0.2))
}
