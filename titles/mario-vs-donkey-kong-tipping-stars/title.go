// Package mvdk is Mario vs. Donkey Kong: Tipping Stars (Wii U 0005000010149300, 3DS
// 000400000012C800; one server for both, the game is cross-buy) on the shared NEX host.
//
// Written for OpenPak on nex-go v2 and the common DataStore protocol, replacing Pretendo's
// nex-go v1 server (AGPL-3.0), which it mirrors in behaviour: the title only uses DataStore.
// Player profiles live in persistence slot 0 (data type 122), tip buckets in slot 1 (data type
// 123), both as meta-binary objects with no file body; three rating slots on a tip bucket carry
// extra-tip total, extra-tip count and play count. Level sharing itself goes through Miiverse.
// Consoles log in through nn-account with their NEX password, so it is a Run-shaped title.
package mvdk

import nexhost "github.com/openpak/nex"

// Title is this game's entry in the shared NEX host.
var Title = nexhost.Title{Name: "mario-vs-donkey-kong-tipping-stars", Defaults: defaults, Run: Run}

// defaults are the title's wire facts: NEX 3.7.1 over plain PRUDP (UDP). The access key is
// configuration (NEX_ACCESS_KEY), recorded in README.md, never a literal here.
func defaults(c *nexhost.Config) {
	c.Transport = "udp"
	c.Version = "3.7.1"
	c.StationScheme = "prudp"
}
