package ui

import (
	"testing"

	"github.com/bjarneo/cliamp/theme"
)

func TestApplyThemeColorsSetsSpectrumFromTheme(t *testing.T) {
	t.Cleanup(func() { ApplyThemeColors(theme.Default()) })

	ApplyThemeColors(theme.Theme{
		Name:     "test",
		Accent:   "#111111",
		BrightFG: "#222222",
		FG:       "#333333",
		Green:    "#a6e3a1",
		Yellow:   "#f9e2af",
		Red:      "#f38ba8",
	})

	if got := specLowStyle.Render("x"); got == "x" {
		t.Fatal("specLowStyle did not apply spectrum low color")
	}
	if got := specMidStyle.Render("x"); got == "x" {
		t.Fatal("specMidStyle did not apply spectrum mid color")
	}
	if got := specHighStyle.Render("x"); got == "x" {
		t.Fatal("specHighStyle did not apply spectrum high color")
	}
	if specLowPrefix == "" || specMidPrefix == "" || specHighPrefix == "" {
		t.Fatal("refreshSpecANSI did not rebuild cached spectrum prefixes")
	}
}
