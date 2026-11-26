package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Stratovarius93/gitree/internal/git"
	"github.com/Stratovarius93/gitree/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

const usage = `gitree - a GitKraken-style commit graph for the terminal

Usage:
  gitree [path]     open the repository at path (default: current directory)
  gitree --version  print the version
  gitree --help     show this help

Press ? inside gitree to see all keys.
`

func main() {
	path := "."
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Println("gitree", version)
			return
		case "-h", "--help", "help":
			fmt.Print(usage)
			return
		}
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
	if _, err := tea.NewProgram(ui.New(repo, ui.RealSystem{}).EnableAutoFetch().EnablePolling().EnableSpinner(), tea.WithAltScreen(), tea.WithReportFocus()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "gitree: %v\n", err)
		os.Exit(1)
	}
}
