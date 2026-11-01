package nex_datastore_super_mario_maker

import (
	"strconv"

	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/openpak/nex/titles/super-mario-maker/nex/datastore/super-mario-maker/constants"
)

// Difficulty is an in-game difficulty band. The game does not send the name: it sends the band's
// failure rate range in the extraData of the course searches.
type Difficulty string

const (
	DifficultyEasy        Difficulty = "easy"
	DifficultyNormal      Difficulty = "normal"
	DifficultyExpert      Difficulty = "expert"
	DifficultySuperExpert Difficulty = "super_expert"
)

// difficultyBands is Course World's banding (constants.COURSE_WORLD_*).
var difficultyBands = []struct {
	Name     Difficulty
	Min, Max int
}{
	{DifficultyEasy, int(constants.COURSE_WORLD_EASY_FAILURE_RATE_MIN), int(constants.COURSE_WORLD_EASY_FAILURE_RATE_MAX)},
	{DifficultyNormal, int(constants.COURSE_WORLD_NORMAL_FAILURE_RATE_MIN), int(constants.COURSE_WORLD_NORMAL_FAILURE_RATE_MAX)},
	{DifficultyExpert, int(constants.COURSE_WORLD_EXPERT_FAILURE_RATE_MIN), int(constants.COURSE_WORLD_EXPERT_FAILURE_RATE_MAX)},
	{DifficultySuperExpert, int(constants.COURSE_WORLD_SUPER_EXPERT_FAILURE_RATE_MIN), int(constants.COURSE_WORLD_SUPER_EXPERT_FAILURE_RATE_MAX)},
}

// DifficultyOf is the band of a failure rate in percent, as returned by
// datastore_smm_db.FailureRate. A course nobody attempted (-1) has no difficulty: "".
func DifficultyOf(failureRate int) Difficulty {
	if failureRate < 0 {
		return ""
	}
	for _, b := range difficultyBands {
		if failureRate >= b.Min && failureRate <= b.Max {
			return b.Name
		}
	}
	return DifficultySuperExpert
}

// FailureRange returns a band's failure rate range, and false for an unknown name.
func FailureRange(d Difficulty) (min, max int, ok bool) {
	for _, b := range difficultyBands {
		if b.Name == d {
			return b.Min, b.Max, true
		}
	}
	return 0, 0, false
}

// ParseDifficultyExtra reads the extraData of RecommendedCourseSearchObject:
//
//	Course World (All)          ["",  "",   "",    "0", "0"]
//	Course World (Easy)         ["1", "0",  "34",  "0", "0"]
//	Course World (Normal)       ["1", "35", "74",  "0", "0"]
//	Course World (Expert)       ["1", "75", "95",  "0", "0"]
//	Course World (Super Expert) ["1", "96", "100", "0", "0"]
//
// (the table is Pretendo's, from packet dumps). Index 0 switches the filter on, 1 and 2 are the
// inclusive failure rate range. Anything else, or a malformed range, means no filter.
func ParseDifficultyExtra(extra types.List[types.String]) (min, max int, ok bool) {
	if len(extra) < 3 || string(extra[0]) != "1" {
		return 0, 0, false
	}

	lo, err1 := strconv.Atoi(string(extra[1]))
	hi, err2 := strconv.Atoi(string(extra[2]))
	if err1 != nil || err2 != nil || lo < 0 || hi > 100 || lo > hi {
		return 0, 0, false
	}

	return lo, hi, true
}
