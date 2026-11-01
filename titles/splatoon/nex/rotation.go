package nex

// The stage rotation as the matchmaking server sees it.
//
// The rotation itself is made by Rotationator and reaches consoles through BOSS
// (schdat2 / VSSetting.byaml). Consoles pick their stages from that file, so the NEX
// side has one job: never let a lobby that was formed for one rotation phase take
// players into the next. The game server reads the same schedule Rotationator wrote
// (schedule.json, the JSON twin of the BYAML) and, at every phase boundary, closes
// participation on the lobbies of the finished phase. Players then form new lobbies
// whose members all hold the new phase's stages and rules.
//
// The schedule file is optional. Without it the server behaves as upstream does, with
// no phase handling at all; the rotation is off until Rotationator has produced one
// (it waits on the console-dumped BOSS keys, see rotationator docs/deploy.md).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Rule names as the BYAML carries them.
const (
	RuleTurfWar       = "cPnt"
	RuleSplatZones    = "cVar"
	RuleTowerCtrl     = "cVlf"
	RuleRainmaker     = "cVgl"
	gameModeRegular   = 0
	gameModeRanked    = 1
	gameModeSplatfest = 12
)

// Phase is one rotation slot.
type Phase struct {
	Start, End    time.Time
	RegularRule   string
	RegularStages []int
	GachiRule     string
	GachiStages   []int
}

// Schedule is a parsed VSSetting.byaml.json.
type Schedule struct {
	Phases []Phase
}

type rawStage struct {
	MapID int `json:"MapID"`
}

type rawPhase struct {
	Time          int        `json:"Time"`
	RegularRule   string     `json:"RegularRule"`
	RegularStages []rawStage `json:"RegularStages"`
	GachiRule     string     `json:"GachiRule"`
	GachiStages   []rawStage `json:"GachiStages"`
}

type rawSchedule struct {
	DateTime string     `json:"DateTime"`
	Phases   []rawPhase `json:"Phases"`
}

func stageIDs(s []rawStage) []int {
	out := make([]int, 0, len(s))
	for _, st := range s {
		out = append(out, st.MapID)
	}
	return out
}

// ParseSchedule reads Rotationator's schedule JSON. The base time is the schedule's
// DateTime; each phase lasts its Time in hours.
func ParseSchedule(data []byte) (*Schedule, error) {
	var raw rawSchedule
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("rotation schedule: %w", err)
	}
	base, err := time.Parse(time.RFC3339, raw.DateTime)
	if err != nil {
		return nil, fmt.Errorf("rotation schedule DateTime %q: %w", raw.DateTime, err)
	}
	if len(raw.Phases) == 0 {
		return nil, errors.New("rotation schedule has no phases")
	}
	s := &Schedule{}
	at := base.UTC()
	for i, p := range raw.Phases {
		if p.Time <= 0 {
			return nil, fmt.Errorf("rotation schedule phase %d has length %d", i, p.Time)
		}
		end := at.Add(time.Duration(p.Time) * time.Hour)
		s.Phases = append(s.Phases, Phase{
			Start: at, End: end,
			RegularRule: p.RegularRule, RegularStages: stageIDs(p.RegularStages),
			GachiRule: p.GachiRule, GachiStages: stageIDs(p.GachiStages),
		})
		at = end
	}
	return s, nil
}

// At returns the phase running at t and its index.
func (s *Schedule) At(t time.Time) (Phase, int, bool) {
	for i, p := range s.Phases {
		if !t.Before(p.Start) && t.Before(p.End) {
			return p, i, true
		}
	}
	return Phase{}, -1, false
}

// Stages returns the stages and rule a lobby of the given game mode plays in the phase.
// Regular battle is Turf War on the regular stages; ranked is the phase's ranked rule on
// the ranked stages. Other modes (private battle, Splatfest) have no rotation.
func (p Phase) Stages(gameMode uint32) (rule string, stages []int, ok bool) {
	switch gameMode {
	case gameModeRegular:
		return p.RegularRule, p.RegularStages, true
	case gameModeRanked:
		return p.GachiRule, p.GachiStages, true
	}
	return "", nil, false
}

// rotationSource keeps the schedule loaded and reloads it when the file changes.
type rotationSource struct {
	path string

	mu       sync.Mutex
	mod      time.Time
	schedule *Schedule
	warned   bool
}

func newRotationSource(path string) *rotationSource { return &rotationSource{path: path} }

// current returns the schedule, or nil when there is none (or it does not parse). A bad
// file keeps the previous good schedule.
func (r *rotationSource) current() *Schedule {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, err := os.Stat(r.path)
	if err != nil {
		return r.schedule
	}
	if st.ModTime().Equal(r.mod) && r.schedule != nil {
		return r.schedule
	}
	data, err := os.ReadFile(r.path)
	if err == nil {
		var s *Schedule
		if s, err = ParseSchedule(data); err == nil {
			r.schedule, r.mod = s, st.ModTime()
			return s
		}
	}
	if !r.warned {
		r.warned = true
		fmt.Fprintf(os.Stderr, "splatoon rotation: %v\n", err)
	}
	return r.schedule
}
