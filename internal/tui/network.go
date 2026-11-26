package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"time"
	"github.com/MuZiJin701/wx-mini-video/internal/app"
)

type networkMsg struct {
	generation int
	err        error
}
type networkStoppedMsg struct{ err error }

func watchNetwork(runtime *app.Runtime, generation int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(2 * time.Second)
		return networkMsg{generation: generation, err: runtime.CheckNetwork()}
	}
}
