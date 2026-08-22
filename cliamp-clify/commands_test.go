package main

import (
	"testing"

	"github.com/urfave/cli/v3"
)

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
