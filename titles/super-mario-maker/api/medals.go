package api

import "github.com/openpak/nex/titles/super-mario-maker/nex/datastore/super-mario-maker/constants"

// medalThresholds are the star counts for medals 1 to 10. The game computes a maker's medals
// itself from its total stars and these numbers, which the server sends in GetApplicationConfig
// (applicationId 0, entries 0-9). The server does not store a medal anywhere.
var medalThresholds = [10]int32{
	constants.STARS_1ST_MEDAL, constants.STARS_2ND_MEDAL, constants.STARS_3RD_MEDAL,
	constants.STARS_4TH_MEDAL, constants.STARS_5TH_MEDAL, constants.STARS_6TH_MEDAL,
	constants.STARS_7TH_MEDAL, constants.STARS_8TH_MEDAL, constants.STARS_9TH_MEDAL,
	constants.STARS_10TH_MEDAL,
}

// Medals is how many maker medals (0 to 10) a total of stars earns.
func Medals(stars int) int {
	n := 0
	for _, t := range medalThresholds {
		if stars >= int(t) {
			n++
		}
	}
	return n
}
