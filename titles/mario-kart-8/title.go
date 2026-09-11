// Package mk8wiiu is Mario Kart 8 (Wii U, 000500001010EB00) on the shared NEX host.
//
// Written for OpenPak on nex-go and the common protocol set; the title-specific parts are
// ported from Pretendo's Python server (AGPL-3.0): tournaments (SimpleSearchObject), the
// competition ranking methods, and the DataStore layout for ghosts and Mario Kart TV. It is a
// Run-shaped title: Wii U consoles log in through nn-account over gRPC with their NEX password,
// not with a Switch ticket, so it owns its two UDP listeners.
package mk8wiiu

import nexhost "github.com/openpak/nex"

// Title is this game's entry in the shared NEX host.
var Title = nexhost.Title{Name: "mario-kart-8", Defaults: defaults, Run: Run}

// defaults are Mario Kart 8's wire facts: NEX 3.5.4 over plain PRUDP (UDP), prudp station URLs.
// The access key stays out of source (NEX_ACCESS_KEY; it is in the title's provenance notes).
func defaults(c *nexhost.Config) {
	c.Transport = "udp"
	c.Version = "3.5.4"
	c.StationScheme = "prudp"
}
