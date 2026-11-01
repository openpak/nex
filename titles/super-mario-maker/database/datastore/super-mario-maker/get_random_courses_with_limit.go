package datastore_smm_db

import (
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_super_mario_maker_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/super-mario-maker/types"
)

// GetRandomCoursesWithLimit returns random completed courses. Courses nobody has starred yet
// are included (they have no ranking row), and only course-typed objects are, so the maker
// objects and the event course metadata file can never be served as a course (Pretendo issue 21).
func GetRandomCoursesWithLimit(limit int) (types.List[datastore_super_mario_maker_types.DataStoreCustomRankingResult], *nex.Error) {
	return QueryCourses(CourseFilter{Random: true, Limit: limit})
}
