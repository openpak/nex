// Package titles is the registry of every game the shared NEX binary can host.
package titles

import (
	nexhost "github.com/openpak/nex"

	acnh "github.com/openpak/nex/titles/animal-crossing-new-horizons"
	arms "github.com/openpak/nex/titles/arms"
	lm3 "github.com/openpak/nex/titles/luigis-mansion-3"
	golf "github.com/openpak/nex/titles/mario-golf-super-rush"
	mk8d "github.com/openpak/nex/titles/mario-kart-8-deluxe"
	mps "github.com/openpak/nex/titles/mario-party-superstars"
	strikers "github.com/openpak/nex/titles/mario-strikers"
	mta "github.com/openpak/nex/titles/mario-tennis-aces"
	minecraft "github.com/openpak/nex/titles/minecraft"
	mhgu "github.com/openpak/nex/titles/monster-hunter-generations-ultimate"
	splatoon2 "github.com/openpak/nex/titles/splatoon-2"
	smb35 "github.com/openpak/nex/titles/super-mario-bros-35"
	smm2 "github.com/openpak/nex/titles/super-mario-maker-2"
	smo "github.com/openpak/nex/titles/super-mario-odyssey"
	ssbu "github.com/openpak/nex/titles/super-smash-bros-ultimate"
)

// All is every title, by the NEX_TITLE value that selects it. Adding a game is one line here.
var All = index(
	acnh.Title,
	arms.Title,
	lm3.Title,
	golf.Title,
	mk8d.Title,
	mps.Title,
	strikers.Title,
	mta.Title,
	minecraft.Title,
	mhgu.Title,
	splatoon2.Title,
	smb35.Title,
	smm2.Title,
	smo.Title,
	ssbu.Title,
)

func index(titles ...nexhost.Title) map[string]nexhost.Title {
	m := make(map[string]nexhost.Title, len(titles))
	for _, t := range titles {
		if _, dup := m[t.Name]; dup {
			panic("nex: two titles named " + t.Name)
		}
		m[t.Name] = t
	}
	return m
}
