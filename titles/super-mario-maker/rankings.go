package smmwiiu

import (
	"database/sql"
	"fmt"

	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/internal/rankingsapi"
)

// Board ids of Super Mario Maker's rankings contract.
const (
	boardCourseStars = 1 // each course's star count (custom ranking 0), the maker owning it
	boardMakerStars  = 2 // a maker's stars summed over their courses
	boardFastClears  = 3 // the best clear time of every course (course records)
)

const (
	courseOK = `o.upload_completed AND NOT o.deleted AND NOT o.under_review AND o.data_type > 2 AND o.data_type < 50`
)

// rankings serves what the game's own lists are built from, all in the title's datastore
// schema: custom ranking 0 (stars) per course and per maker, and the course records.
func rankings(db *sql.DB) rankingsapi.Source {
	names := map[int64]string{boardCourseStars: "Course stars", boardMakerStars: "Maker stars", boardFastClears: "Fastest clears"}
	name := func(id int64) string { return names[id] }
	exists := func(id int, desc bool, from string) string {
		return fmt.Sprintf(`SELECT '%d', %t WHERE EXISTS (SELECT 1 FROM %s)`, id, desc, from)
	}
	return nexhost.CombineRankings(
		nexhost.SQLRankings{
			DB: db, Kind: rankingsapi.KindScore, Name: name,
			BoardsSQL: exists(boardCourseStars, true, `datastore.object_custom_rankings WHERE application_id = 0`),
			From:      `datastore.object_custom_rankings st JOIN datastore.objects o ON o.data_id = st.data_id`,
			Match:     `$1::bigint = 1 AND st.application_id = 0 AND ` + courseOK,
			PID:       `o.owner`, Score: `st.value`, Updated: `o.update_date AT TIME ZONE 'UTC'`,
			Desc: `true`, Tie: `o.update_date ASC, o.data_id ASC`,
		},
		nexhost.SQLRankings{
			DB: db, Kind: rankingsapi.KindScore, Name: name,
			BoardsSQL: exists(boardMakerStars, true, `datastore.object_custom_rankings WHERE application_id = 0`),
			From: `(SELECT o.owner AS pid, SUM(st.value) AS stars, MAX(o.update_date) AS upd
				FROM datastore.object_custom_rankings st JOIN datastore.objects o ON o.data_id = st.data_id
				WHERE st.application_id = 0 AND ` + courseOK + ` GROUP BY o.owner) m`,
			Match: `$1::bigint = 2`,
			PID:   `m.pid`, Score: `m.stars`, Updated: `m.upd AT TIME ZONE 'UTC'`,
			Desc: `true`, Tie: `m.upd ASC, m.pid ASC`,
		},
		nexhost.SQLRankings{
			DB: db, Kind: rankingsapi.KindTime, Name: name,
			BoardsSQL: exists(boardFastClears, false, `datastore.course_records WHERE best_pid IS NOT NULL`),
			From:      `datastore.course_records r JOIN datastore.objects o ON o.data_id = r.data_id`,
			Match:     `$1::bigint = 3 AND r.best_pid IS NOT NULL AND ` + courseOK,
			PID:       `r.best_pid`, Score: `r.best_score`, Updated: `r.update_date AT TIME ZONE 'UTC'`,
			Desc: `false`, Tie: `r.update_date ASC, r.data_id ASC, r.slot ASC`,
		},
	)
}
