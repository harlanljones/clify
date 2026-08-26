package main

import (
	"testing"

	"github.com/urfave/cli/v3"
)

func TestVersionCommandWiring(t *testing.T) {
	cmd := versionCommand()

	if cmd == nil || cmd.Name != "version" {
		t.Fatalf("version command = %#v, want a subcommand named 'version'", cmd)
	}
	if cmd.Usage == "" {
		t.Fatal("version command missing Usage")
	}
	var registered bool
	for _, sub := range buildApp().Commands {
		if sub.Name == "version" {
			registered = true
			break
		}
	}
	if !registered {
		t.Fatal("version subcommand not registered on the root command")
	}
}

func TestSpotifyLoginCommandWiring(t *testing.T) {
	cmd := spotifyCommand()

	var login *cli.Command
	for _, sub := range cmd.Commands {
		if sub.Name == "login" {
			login = sub
			break
		}
	}
	if login == nil {
		t.Fatalf("spotify login subcommand missing; got %d subcommands", len(cmd.Commands))
	}

	found := false
	for _, f := range login.Flags {
		for _, name := range f.Names() {
			if name == "client-id" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("spotify login missing --client-id flag")
	}
}
