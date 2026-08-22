package model

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/playlist"
)

func TestGlobalHelpOpensOverActiveTextInput(t *testing.T) {
	m := Model{search: searchState{active: true, query: "jazz"}}

	m.handleKey(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if !m.keymap.visible {
		t.Fatal("keymap.visible = false after Ctrl+K, want true")
	}
	if !m.search.active || m.search.query != "jazz" {
		t.Fatalf("search state = %+v, want active input preserved", m.search)
	}

	m.handleKey(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if m.keymap.visible {
		t.Fatal("keymap.visible = true after second Ctrl+K, want false")
	}
	if !m.search.active || m.search.query != "jazz" {
		t.Fatalf("search state = %+v after closing help, want active input preserved", m.search)
	}
}

func TestLyricsRetryStartsNewRequest(t *testing.T) {
	p := playlist.New()
	p.Add(playlist.Track{Artist: "Artist", Title: "Title"})
	m := Model{
		playlist: p,
		lyrics: lyricsState{
			visible: true,
			err:     errors.New("temporary failure"),
		},
	}

	if cmd := m.handleKey(tea.KeyPressMsg{Text: "r"}); cmd == nil {
		t.Fatal("lyrics retry command is nil")
	}
	if !m.lyrics.loading {
		t.Fatal("lyrics.loading = false after retry")
	}
	if m.lyrics.err != nil {
		t.Fatalf("lyrics.err = %v after retry, want nil", m.lyrics.err)
	}
	if m.lyrics.query != "Artist\nTitle" {
		t.Fatalf("lyrics.query = %q, want lookup key", m.lyrics.query)
	}
}

func TestUndoRestoresClearedQueue(t *testing.T) {
	p := playlist.New()
	p.Add(playlist.Track{Title: "One"}, playlist.Track{Title: "Two"})
	p.Queue(0)
	p.Queue(1)
	m := Model{playlist: p, queue: queueOverlay{visible: true}}

	m.handleQueueKey(tea.KeyPressMsg{Text: "c"})
	if got := p.QueueLen(); got != 0 {
		t.Fatalf("queue length after clear = %d, want 0", got)
	}
	m.handleKey(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	if got := p.QueueTracks(); len(got) != 2 || got[0].Title != "One" || got[1].Title != "Two" {
		t.Fatalf("queue after undo = %#v, want original queue", got)
	}
}
