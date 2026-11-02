package splatoonwiiu

import (
	"database/sql"
	"fmt"

	nexhost "github.com/openpak/nex"
)

// festRankings is one board per Splatfest over ranking_splatoon.user_scores: each player's
// best score in that festival, highest first, the order the game's own score list uses.
func festRankings(db *sql.DB) nexhost.SQLRankings {
	return nexhost.SQLRankings{
		DB:        db,
		BoardsSQL: `SELECT splatfest_id::text, true FROM ranking_splatoon.user_scores GROUP BY splatfest_id ORDER BY splatfest_id`,
		Kind:      "score",
		Name:      func(id int64) string { return fmt.Sprintf("Splatfest %d", id) },
		From:      `ranking_splatoon.user_scores`,
		Match:     `splatfest_id = $1`,
		PID:       `uploader_pid`,
		Score:     `score`,
		Updated:   `last_updated`,
		Desc:      `true`,
		Tie:       `last_updated ASC, uploader_pid ASC`,
	}
}
