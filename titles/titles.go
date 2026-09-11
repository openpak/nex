// Package titles is the registry of every game the shared NEX binary can host.
package titles

import (
	nexhost "github.com/openpak/nex"

	// Wii U and 3DS, adapted from Pretendo (Run shape)
	acnl "github.com/openpak/nex/titles/animal-crossing-new-leaf"
	drluigi "github.com/openpak/nex/titles/dr-luigi"
	f12011 "github.com/openpak/nex/titles/f1-2011"
	ironfall "github.com/openpak/nex/titles/ironfall-invasion"
	lm2 "github.com/openpak/nex/titles/luigis-mansion-2"
	mpff "github.com/openpak/nex/titles/metroid-prime-federation-force"
	miraclecure "github.com/openpak/nex/titles/miracle-cure"
	mh4u "github.com/openpak/nex/titles/monster-hunter-4-ultimate"
	mhgen "github.com/openpak/nex/titles/monster-hunter-generations"
	mhxx "github.com/openpak/nex/titles/monster-hunter-xx"
	pikmin3 "github.com/openpak/nex/titles/pikmin-3"
	pokegen6 "github.com/openpak/nex/titles/pokemon-gen6"
	pokken "github.com/openpak/nex/titles/pokken-tournament"
	ppt "github.com/openpak/nex/titles/puyo-puyo-tetris"
	splatoonwiiu "github.com/openpak/nex/titles/splatoon"
	sdsw "github.com/openpak/nex/titles/steel-diver-sub-wars"
	sm3dwu "github.com/openpak/nex/titles/super-mario-3d-world"
	smmwiiu "github.com/openpak/nex/titles/super-mario-maker"
	ssb3ds "github.com/openpak/nex/titles/super-smash-bros-3ds"
	ssbwiiu "github.com/openpak/nex/titles/super-smash-bros-wiiu"
	ssfiv "github.com/openpak/nex/titles/super-street-fighter-iv"
	wiiuchat "github.com/openpak/nex/titles/wiiu-chat"

	isz "github.com/openpak/nex/titles/ice-station-z"
	kiu "github.com/openpak/nex/titles/kid-icarus-uprising"
	mk7 "github.com/openpak/nex/titles/mario-kart-7"
	minecraftwiiu "github.com/openpak/nex/titles/minecraft-wiiu"
	mh4 "github.com/openpak/nex/titles/monster-hunter-4"
	mhstories "github.com/openpak/nex/titles/monster-hunter-stories"
	prw "github.com/openpak/nex/titles/pokemon-rumble-world"
	puyo20th "github.com/openpak/nex/titles/puyo-puyo-20th"
	remerc3d "github.com/openpak/nex/titles/resident-evil-the-mercenaries-3d"
	tkcd "github.com/openpak/nex/titles/team-kirby-clash-deluxe"
	tetrisaxis "github.com/openpak/nex/titles/tetris-axis"
	tfh "github.com/openpak/nex/titles/tri-force-heroes"
	ykw2 "github.com/openpak/nex/titles/yo-kai-watch-2"
	ykwb "github.com/openpak/nex/titles/yo-kai-watch-blasters"

	// Switch (Register shape)
	acnh "github.com/openpak/nex/titles/animal-crossing-new-horizons"
	arms "github.com/openpak/nex/titles/arms"
	kdb "github.com/openpak/nex/titles/kirbys-dream-buffet"
	lm3 "github.com/openpak/nex/titles/luigis-mansion-3"
	golf "github.com/openpak/nex/titles/mario-golf-super-rush"
	mk8d "github.com/openpak/nex/titles/mario-kart-8-deluxe"
	mps "github.com/openpak/nex/titles/mario-party-superstars"
	strikers "github.com/openpak/nex/titles/mario-strikers"
	mta "github.com/openpak/nex/titles/mario-tennis-aces"
	minecraft "github.com/openpak/nex/titles/minecraft"
	mhgu "github.com/openpak/nex/titles/monster-hunter-generations-ultimate"
	splatoon2 "github.com/openpak/nex/titles/splatoon-2"
	sm3dw "github.com/openpak/nex/titles/super-mario-3d-world-bowsers-fury"
	smb35 "github.com/openpak/nex/titles/super-mario-bros-35"
	smm2 "github.com/openpak/nex/titles/super-mario-maker-2"
	smo "github.com/openpak/nex/titles/super-mario-odyssey"
	ssbu "github.com/openpak/nex/titles/super-smash-bros-ultimate"
)

// All is every title, by the NEX_TITLE value that selects it. Adding a game is one line here.
var All = index(
	isz.Title,
	kiu.Title,
	mk7.Title,
	minecraftwiiu.Title,
	mh4.Title,
	mhstories.Title,
	prw.Title,
	puyo20th.Title,
	remerc3d.Title,
	tkcd.Title,
	tetrisaxis.Title,
	tfh.Title,
	ykw2.Title,
	ykwb.Title,
	acnl.Title,
	drluigi.Title,
	f12011.Title,
	ironfall.Title,
	lm2.Title,
	mpff.Title,
	miraclecure.Title,
	mh4u.Title,
	mhgen.Title,
	mhxx.Title,
	pikmin3.Title,
	pokegen6.Title,
	pokken.Title,
	ppt.Title,
	splatoonwiiu.Title,
	sdsw.Title,
	sm3dwu.Title,
	smmwiiu.Title,
	ssb3ds.Title,
	ssbwiiu.Title,
	ssfiv.Title,
	wiiuchat.Title,
	acnh.Title,
	arms.Title,
	lm3.Title,
	golf.Title,
	kdb.Title,
	mk8d.Title,
	mps.Title,
	strikers.Title,
	mta.Title,
	minecraft.Title,
	mhgu.Title,
	splatoon2.Title,
	sm3dw.Title,
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
