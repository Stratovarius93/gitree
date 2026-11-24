package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	path := "."
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	repo, err := git.Open(path)
	if err != nil {
		if errors.Is(err, git.ErrNotRepo) {
			fmt.Fprintf(os.Stderr, "gitree: not a git repository: %s\n", path)
		} else {
			fmt.Fprintf(os.Stderr, "gitree: %v\n", err)
		}
		os.Exit(1)
	}
	if _, err := tea.NewProgram(ui.New(repo, ui.RealSystem{}).EnableAutoFetch(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "gitree: %v\n", err)
		os.Exit(1)
	}
}
