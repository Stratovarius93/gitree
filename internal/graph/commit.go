// Adapted from github.com/jesseduffield/lazygit pkg/gui/presentation/graph (MIT). See LICENSE in this directory.

package graph

import "sync"

// StringPool interns strings so that equal hashes share one pointer; the graph
// compares hashes by pointer identity.
type StringPool struct {
	sync.Map
}

func (p *StringPool) Add(s string) *string {
	entry, _ := p.LoadOrStore(s, &s)
	return entry.(*string)
}

// Commit is the minimal commit shape the graph renderer needs.
type Commit struct {
	hash    *string
	parents []*string
}

type CommitOpts struct {
	Hash    string
	Parents []string
}

func NewCommit(pool *StringPool, opts CommitOpts) *Commit {
	parents := make([]*string, len(opts.Parents))
	for i, p := range opts.Parents {
		parents[i] = pool.Add(p)
	}
	return &Commit{hash: pool.Add(opts.Hash), parents: parents}
}

// NewCommits builds commits sharing one string pool, so equal hashes share
// pointers.
func NewCommits(opts []CommitOpts) []*Commit {
	pool := &StringPool{}
	commits := make([]*Commit, len(opts))
	for i, o := range opts {
		commits[i] = NewCommit(pool, o)
	}
	return commits
}

func (c *Commit) Hash() string { return *c.hash }

func (c *Commit) HashPtr() *string { return c.hash }

func (c *Commit) Parents() []string {
	out := make([]string, len(c.parents))
	for i, p := range c.parents {
		out[i] = *p
	}
	return out
}

func (c *Commit) ParentPtrs() []*string { return c.parents }

func (c *Commit) IsMerge() bool { return len(c.parents) > 1 }

func (c *Commit) IsFirstCommit() bool { return len(c.parents) == 0 }
