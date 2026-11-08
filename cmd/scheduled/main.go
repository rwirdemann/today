// Command scheduled shows today's tasks from an Obsidian zettelkasten in a TUI.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rwirdemann/scheduled/internal/tui"
)

func main() {
	home, _ := os.UserHomeDir()
	defVault := filepath.Join(home, "Dropbox", "zettelkasten")
	if v := os.Getenv("ZETTEL_VAULT"); v != "" {
		defVault = v
	}
	vault := flag.String("vault", defVault, "path to the zettelkasten (env ZETTEL_VAULT)")
	folder := flag.String("folder", "Tagebuch", "daily notes folder inside the vault")
	flag.Parse()

	dir := filepath.Join(*vault, *folder)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "daily notes folder not found: %s\n", dir)
		os.Exit(1)
	}

	m, err := tui.New(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer m.Close()

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
