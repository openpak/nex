package titles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Wii U titles that count online sessions; every other session title below is a 3DS one.
var sessionWiiU = map[string]bool{
	"bayonetta-2": true, "dr-luigi": true, "monster-hunter-3-ultimate": true, "minecraft-wiiu": true,
	"pokken-tournament": true, "puyo-puyo-tetris": true, "sonic-all-stars-racing-transformed-wiiu": true, "terraria": true,
}

var sessionTitles = []string{
	"animal-crossing-new-leaf", "bayonetta-2", "dr-luigi", "f1-2011", "ice-station-z", "ironfall-invasion",
	"kid-icarus-uprising", "luigis-mansion-2", "metroid-prime-federation-force", "minecraft-wiiu",
	"monster-hunter-3-ultimate", "monster-hunter-4", "monster-hunter-4-ultimate", "monster-hunter-generations",
	"monster-hunter-stories", "monster-hunter-xx", "pokemon-gen6", "pokken-tournament", "puyo-puyo-20th",
	"puyo-puyo-tetris", "resident-evil-the-mercenaries-3d", "sonic-all-stars-racing-transformed-wiiu",
	"steel-diver-sub-wars", "super-street-fighter-iv", "terraria", "tetris-axis", "tri-force-heroes",
	"yo-kai-watch-2", "yo-kai-watch-blasters",
}

// Titles with no gathering matchmaking: no session metric, on purpose.
var noSessions = []string{"miracle-cure", "wiiu-chat", "pikmin-3", "super-mario-3d-world", "super-mario-maker",
	"team-kirby-clash-deluxe", "pokemon-rumble-world", "mario-vs-donkey-kong-tipping-stars"}

// Titles whose Ranking protocol is backed by the shared scoreboard store and served on the contract.
var scoreboardTitles = []string{"tetris-axis", "puyo-puyo-20th", "pokken-tournament", "super-smash-bros-3ds",
	"super-smash-bros-wiiu", "yo-kai-watch-2", "monster-hunter-4", "monster-hunter-stories", "monster-hunter-xx",
	"ice-station-z", "resident-evil-the-mercenaries-3d"}

func read(t *testing.T, dir string) (title, all string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "title.go"))
	if err != nil {
		t.Fatal(err)
	}
	title = string(b)
	filepath.Walk(dir, func(p string, i os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			if b, err := os.ReadFile(p); err == nil {
				all += string(b)
			}
		}
		return nil
	})
	return title, all
}

func TestSessionMetricsWired(t *testing.T) {
	for _, d := range sessionTitles {
		title, all := read(t, d)
		ns := "3ds"
		if sessionWiiU[d] {
			ns = "wiiu"
		}
		if want := `nexhost.WatchSessions(cfg, "game_sessions.` + d + `", "` + ns + `")`; !strings.Contains(title, want) {
			t.Errorf("%s: want %s in title.go", d, want)
		}
		if !strings.Contains(all, "common_globals.NewMatchmakingManager") {
			t.Errorf("%s: not on the common Postgres matchmaking manager", d)
		}
	}
	for _, d := range noSessions {
		if title, _ := read(t, d); strings.Contains(title, "WatchSessions") {
			t.Errorf("%s has no gathering matchmaking but wires WatchSessions", d)
		}
	}
}

func TestScoreboardTitlesPersistAndServe(t *testing.T) {
	for _, d := range scoreboardTitles {
		title, all := read(t, d)
		if !strings.Contains(all, "scoreboard.Install(") {
			t.Errorf("%s: Ranking has no scoreboard store", d)
		}
		if !strings.Contains(title, "nexhost.ServeRankings(nexhost.ScoresRankings(") {
			t.Errorf("%s: rankings contract not served", d)
		}
	}
}
