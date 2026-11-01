package nex

// Splatoon-specific matchmaking, layered on the common MatchmakeExtension. The common
// library already does attribute-range search and the selection methods the game asks
// for (random, nearest neighbour on attribute 1, broaden range); what lives here is what
// is particular to this title:
//
//   - search criteria cleanup (attribute 4 is not part of the match, upstream's rule);
//   - Splatfest lobbies stay open for the opposing team (upstream feat/fests);
//   - lobbies do not outlive their rotation phase (rotation.go);
//   - an optional trace of what the game asks for, for working out the attribute layout.

import (
	"database/sql"
	"os"
	"sync"
	"time"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/v2/match-making/types"
	"github.com/openpak/nex/titles/splatoon/globals"
)

// traceMatchmaking logs the shape of every auto-matchmake request: game mode, attribute
// values, criteria count. No pids or names. Off unless SPLATOON_TRACE_MATCHMAKING=1.
var traceMatchmaking = os.Getenv("SPLATOON_TRACE_MATCHMAKING") == "1"

func cleanupMatchmakeSessionSearchCriteriasHandler(searchCriterias types.List[match_making_types.MatchmakeSessionSearchCriteria]) {
	for _, searchCriteria := range searchCriterias {
		if len(searchCriteria.Attribs) > 4 {
			searchCriteria.Attribs[4] = types.NewString("")
		}
	}
	if traceMatchmaking {
		for i, c := range searchCriterias {
			globals.Logger.Infof("Matchmaking trace: criteria %d gameMode=%q attribs=%q selection=%d vacant=%d",
				i, string(c.GameMode), attribStrings(c.Attribs), c.SelectionMethod, c.VacantParticipants)
		}
	}
}

func attribStrings(a types.List[types.String]) []string {
	out := make([]string, len(a))
	for i, s := range a {
		out[i] = string(s)
	}
	return out
}

// openFestLobbies opens participation on the Splatfest lobby (game mode 12) that pid is
// in. The game creates its Splatfest lobby closed and expects the server to open it so
// the opposing team's four can be brought in.
const openFestLobbySQL = `UPDATE matchmaking.matchmake_sessions ms SET open_participation = true
	FROM matchmaking.gatherings g
	WHERE g.id = ms.id AND g.registered = true AND g.type = 'MatchmakeSession'
	AND ms.game_mode = $1 AND ms.open_participation = false AND $2 = ANY(g.participants)`

func openFestLobby(db *sql.DB, mu sync.Locker, pid uint64) (int64, error) {
	mu.Lock()
	defer mu.Unlock()
	res, err := db.Exec(openFestLobbySQL, gameModeSplatfest, int64(pid))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func onAfterAutoMatchmakeWithParamPostpone(packet nex.PacketInterface, param match_making_types.AutoMatchmakeParam) {
	if traceMatchmaking {
		src := param.SourceMatchmakeSession
		globals.Logger.Infof("Matchmaking trace: auto-matchmake gameMode=%d attribs=%v criteria=%d additional=%d gidCheck=%d",
			uint32(src.GameMode), src.Attributes, len(param.LstSearchCriteria), len(param.AdditionalParticipants), uint32(param.GIDForParticipationCheck))
	}
	if uint32(param.SourceMatchmakeSession.GameMode) != gameModeSplatfest {
		return
	}
	n, err := openFestLobby(globals.MatchmakingManager.Database, globals.MatchmakingManager.Mutex, uint64(packet.Sender().PID()))
	if err != nil {
		globals.Logger.Errorf("Matchmaking: opening Splatfest lobby failed: %v", err)
		return
	}
	if traceMatchmaking {
		globals.Logger.Infof("Matchmaking trace: opened %d Splatfest lobbies", n)
	}
}

// closeStaleLobbies closes participation on registered regular and ranked lobbies that
// started before cutoff, the start of the current rotation phase.
const closeStaleLobbiesSQL = `UPDATE matchmaking.matchmake_sessions ms SET open_participation = false
	FROM matchmaking.gatherings g
	WHERE g.id = ms.id AND g.registered = true AND g.type = 'MatchmakeSession'
	AND ms.open_participation = true AND ms.game_mode IN ($1, $2) AND g.started_time < $3`

func closeStaleLobbies(db *sql.DB, mu sync.Locker, cutoff time.Time) (int64, error) {
	mu.Lock()
	defer mu.Unlock()
	res, err := db.Exec(closeStaleLobbiesSQL, gameModeRegular, gameModeRanked, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// phaseWatcher notices phase changes and closes the finished phase's lobbies.
type phaseWatcher struct {
	src     *rotationSource
	closeFn func(cutoff time.Time) (int64, error)
	now     func() time.Time
	logf    func(format string, args ...any)

	started bool
	last    int
}

func (w *phaseWatcher) tick() {
	s := w.src.current()
	if s == nil {
		return
	}
	phase, idx, ok := s.At(w.now())
	if !ok || (w.started && idx == w.last) {
		return
	}
	w.started, w.last = true, idx
	n, err := w.closeFn(phase.Start)
	if err != nil {
		w.started = false // try again next tick
		w.logf("Rotation: closing the previous phase's lobbies failed: %v", err)
		return
	}
	w.logf("Rotation: phase %d began, closed %d lobbies from earlier phases", idx, n)
}

func (w *phaseWatcher) run(every time.Duration) {
	for {
		w.tick()
		time.Sleep(every)
	}
}

// startRotation starts the phase watcher when a schedule path is configured.
func startRotation() {
	path := os.Getenv("SPLATOON_ROTATION_FILE")
	if path == "" {
		path = "/rotation/schedule.json"
	}
	w := &phaseWatcher{
		src: newRotationSource(path),
		closeFn: func(cutoff time.Time) (int64, error) {
			m := globals.MatchmakingManager
			return closeStaleLobbies(m.Database, m.Mutex, cutoff)
		},
		now:  time.Now,
		logf: globals.Logger.Infof,
	}
	go w.run(15 * time.Second)
}
