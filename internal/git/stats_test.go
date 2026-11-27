package git

import (
	"testing"

	"github.com/Stratovarius93/gitree/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStats(t *testing.T) {
	tr := testutil.NewRepo(t)
	h1 := tr.Commit("init", map[string]string{"a": "1\n2\n"})
	h2 := tr.Commit("two", map[string]string{"a": "1\n", "b": "x\ny\nz\n"})
	r, _ := Open(tr.Dir)
	st, err := r.Stats([]string{h1, h2})
	require.NoError(t, err)
	assert.Equal(t, Stat{Added: 2}, st[h1], "root commit vs empty tree")
	assert.Equal(t, Stat{Added: 3, Deleted: 1}, st[h2])
}

func TestStatsMergeAndEmpty(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "1\n"})
	tr.Branch("feat")
	tr.Checkout("feat")
	tr.Commit("z", map[string]string{"z": "z\n"})
	tr.Checkout("main")
	tr.Commit("m", map[string]string{"m": "m\n"})
	merge := tr.Merge("feat", "merge")
	empty := tr.Commit("empty", nil)
	bin := tr.Commit("bin", map[string]string{"blob": "\x00\x01\x02"})
	r, _ := Open(tr.Dir)
	st, err := r.Stats([]string{merge, empty, bin})
	require.NoError(t, err)
	for name, h := range map[string]string{"merge": merge, "empty": empty, "binary": bin} {
		s, ok := st[h]
		assert.True(t, ok, "%s commit has an entry", name)
		assert.Equal(t, Stat{}, s, name)
	}
}

func TestStatsNoHashes(t *testing.T) {
	tr := testutil.NewRepo(t)
	tr.Commit("init", map[string]string{"a": "1\n"})
	r, _ := Open(tr.Dir)
	st, err := r.Stats(nil)
	require.NoError(t, err)
	assert.NotNil(t, st)
	assert.Empty(t, st)
}
