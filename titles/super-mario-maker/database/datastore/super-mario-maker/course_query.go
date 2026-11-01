package datastore_smm_db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	datastore_super_mario_maker_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/super-mario-maker/types"
	datastore_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/types"
	"github.com/lib/pq"
	"github.com/openpak/nex/titles/super-mario-maker/database"
	datastore_db "github.com/openpak/nex/titles/super-mario-maker/database/datastore"
	"github.com/openpak/nex/titles/super-mario-maker/globals"
)

// Object rating slots the game keeps on every course. The mapping is what the course archiver
// reads off the real server (Pretendo smm1-course-archive, GPL-3.0, archive.py): plays, an
// unknown counter, clears, attempts, failures, an unknown counter, Miiverse comments.
const (
	RatingSlotPlays          = 0
	RatingSlotClears         = 2
	RatingSlotAttempts       = 3
	RatingSlotFailures       = 4
	RatingSlotMiiverseCounts = 6
)

// Data types: a course is > 2 and < 50 (maker = 1, PrepareAttachFile objects = 2, event
// metadata = 50, event courses = 51). Pretendo issue 21 traces a Course World crash to a query
// that had no such bound.
const (
	courseTypeMin = 2
	courseTypeMax = 50
	makerType     = 1
)

// CourseFilter selects completed, visible course (or maker) objects joined with one custom
// ranking.
type CourseFilter struct {
	// AppID is the custom ranking joined for the score; 0 is the star count.
	AppID uint32
	// Makers lists maker objects instead of courses.
	Makers bool
	// Newest orders by creation date; Random orders randomly; otherwise by score, best first.
	Newest, Random bool
	// ScoreMin/ScoreMax limit the joined score (inclusive) when non-nil.
	ScoreMin, ScoreMax *int64
	// FailMin/FailMax limit the failure rate in percent (inclusive) when non-nil. Courses nobody
	// has attempted have no rate and are left out whenever either is set.
	FailMin, FailMax *int
	// Name limits to courses whose name contains this text, ignoring case.
	Name          string
	Offset, Limit int
}

// IsMakerRanking reports whether a windowed custom ranking ID ranks makers. Windowed IDs are
// type*100000000 + a date; type 4 is the maker star ranking (seen with the real client by
// Protarium's fork of the Pretendo server).
func IsMakerRanking(appID uint32) bool { return appID >= 400000000 && appID < 500000000 }

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// BuildCourseQuery returns the SQL and its arguments for f. It is split from the database call
// so the filter logic can be tested without a database.
func BuildCourseQuery(f CourseFilter) (string, []any) {
	args := []any{f.AppID, f.Limit, f.Offset}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	joins := `
		LEFT JOIN datastore.object_custom_rankings ranking
			ON ranking.data_id = object.data_id AND ranking.application_id = $1`
	where := []string{
		"object.upload_completed = TRUE",
		"object.deleted = FALSE",
		"object.under_review = FALSE",
	}

	if f.Makers {
		where = append(where, fmt.Sprintf("object.data_type = %d", makerType))
	} else {
		where = append(where, fmt.Sprintf("object.data_type > %d AND object.data_type < %d", courseTypeMin, courseTypeMax))
	}

	if f.ScoreMin != nil {
		where = append(where, "COALESCE(ranking.value, 0) >= "+arg(*f.ScoreMin))
	}
	if f.ScoreMax != nil {
		where = append(where, "COALESCE(ranking.value, 0) <= "+arg(*f.ScoreMax))
	}

	if f.FailMin != nil || f.FailMax != nil {
		joins += fmt.Sprintf(`
		LEFT JOIN datastore.object_ratings clears
			ON clears.data_id = object.data_id AND clears.slot = %d
		LEFT JOIN datastore.object_ratings attempts
			ON attempts.data_id = object.data_id AND attempts.slot = %d`, RatingSlotClears, RatingSlotAttempts)
		where = append(where, "COALESCE(attempts.total_value, 0) > 0")
		// * Same rounding as FailureRate
		rate := "(100 - ROUND(COALESCE(clears.total_value, 0) * 100.0 / attempts.total_value))"
		if f.FailMin != nil {
			where = append(where, rate+" >= "+arg(*f.FailMin))
		}
		if f.FailMax != nil {
			where = append(where, rate+" <= "+arg(*f.FailMax))
		}
	}

	if f.Name != "" {
		where = append(where, `object.name ILIKE '%' || `+arg(escapeLike(f.Name))+` || '%' ESCAPE '\'`)
	}

	order := "COALESCE(ranking.value, 0) DESC, object.data_id DESC"
	switch {
	case f.Random:
		order = "RANDOM()"
	case f.Newest:
		order = "object.creation_date DESC, object.data_id DESC"
	}

	return `
		SELECT
			object.data_id,
			object.owner,
			object.size,
			object.name,
			object.data_type,
			object.meta_binary,
			object.permission,
			object.permission_recipients,
			object.delete_permission,
			object.delete_permission_recipients,
			object.period,
			object.refer_data_id,
			object.flag,
			object.tags,
			object.creation_date,
			object.update_date,
			COALESCE(ranking.value, 0)
		FROM datastore.objects object` + joins + `
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY ` + order + `
		LIMIT $2 OFFSET $3`, args
}

// FailureRate is the percentage of attempts that did not clear the course, rounded to the
// nearest whole percent, or -1 when nobody has attempted it. The in-game difficulty bands are
// failure rate bands (constants.COURSE_WORLD_*_FAILURE_RATE_*).
func FailureRate(clears, attempts int64) int {
	if attempts <= 0 {
		return -1
	}
	if clears > attempts {
		clears = attempts
	}
	if clears < 0 {
		clears = 0
	}
	// * Integer round-half-up of clears*100/attempts, matching SQL ROUND on positive numerics
	clearPct := (clears*200 + attempts) / (attempts * 2)
	return int(100 - clearPct)
}

// QueryCourses runs f.
func QueryCourses(f CourseFilter) (types.List[datastore_super_mario_maker_types.DataStoreCustomRankingResult], *nex.Error) {
	query, args := BuildCourseQuery(f)
	courses := types.NewList[datastore_super_mario_maker_types.DataStoreCustomRankingResult]()

	rows, err := database.Postgres.Query(query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return courses, nil
		}

		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	defer rows.Close()

	for rows.Next() {
		course := datastore_super_mario_maker_types.NewDataStoreCustomRankingResult()
		course.MetaInfo = datastore_types.NewDataStoreMetaInfo()
		course.MetaInfo.Permission = datastore_types.NewDataStorePermission()
		course.MetaInfo.DelPermission = datastore_types.NewDataStorePermission()
		course.MetaInfo.ExpireTime = types.NewDateTime(0x9C3F3E0000) // * 9999-12-31T00:00:00.000Z. This is what the real server sends
		course.MetaInfo.Ratings = types.NewList[datastore_types.DataStoreRatingInfoWithSlot]()

		var createdDate time.Time
		var updatedDate time.Time
		var tagArray []string

		err := rows.Scan(
			&course.MetaInfo.DataID,
			&course.MetaInfo.OwnerID,
			&course.MetaInfo.Size,
			&course.MetaInfo.Name,
			&course.MetaInfo.DataType,
			&course.MetaInfo.MetaBinary,
			&course.MetaInfo.Permission.Permission,
			pq.Array(&course.MetaInfo.Permission.RecipientIDs),
			&course.MetaInfo.DelPermission.Permission,
			pq.Array(&course.MetaInfo.DelPermission.RecipientIDs),
			&course.MetaInfo.Period,
			&course.MetaInfo.ReferDataID,
			&course.MetaInfo.Flag,
			pq.Array(&tagArray),
			&createdDate,
			&updatedDate,
			&course.Score,
		)
		if err != nil {
			globals.Logger.Error(err.Error())
			continue
		}

		ratings, nexError := datastore_db.GetObjectRatingsWithSlotByDataID(course.MetaInfo.DataID)
		if nexError != nil {
			return nil, nexError
		}

		course.MetaInfo.Tags = make(types.List[types.String], 0, len(tagArray))
		for i := range tagArray {
			course.MetaInfo.Tags = append(course.MetaInfo.Tags, types.String(tagArray[i]))
		}

		course.MetaInfo.Ratings = ratings

		course.MetaInfo.CreatedTime.FromTimestamp(createdDate)
		course.MetaInfo.UpdatedTime.FromTimestamp(updatedDate)
		course.MetaInfo.ReferredTime.FromTimestamp(createdDate) // * This is what the real server does

		// * Order is 1-based from the offset in the ranking lists; the random lists send 0
		if !f.Random {
			course.Order = types.NewUInt32(uint32(f.Offset + len(courses) + 1))
		}

		courses = append(courses, course)
	}

	if err := rows.Err(); err != nil {
		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	return courses, nil
}

// PreviousRankingPeriod returns the newest custom ranking application ID below appID that
// shares its leading digit (windowed IDs look like <type><8 digit date>) and has rows, so a
// ranking whose current window is still empty can show the last one that was not.
func PreviousRankingPeriod(appID uint32) (uint32, bool) {
	kind := uint64(appID) / 100000000 * 100000000

	var prev sql.NullInt64
	err := database.Postgres.QueryRow(`
		SELECT MAX(application_id) FROM datastore.object_custom_rankings
		WHERE application_id < $1 AND application_id >= $2`, int64(appID), int64(kind)).Scan(&prev)
	if err != nil || !prev.Valid {
		return 0, false
	}

	return uint32(prev.Int64), true
}
