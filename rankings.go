package nexhost

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/openpak/nex/internal/rankingsapi"
)

// RankingsNetwork is the account-core link namespace of every NEX pid on this image: the
// Wii U and the 3DS share it.
const RankingsNetwork = "wiiu"

// ServeRankings starts the rankings contract (internal/rankingsapi) over src on
// RANKINGS_ADDR, for example ":9090". Unset leaves it off. A title calls it once its store
// is open; the title supplies only the Source.
func ServeRankings(src rankingsapi.Source) {
	rankingsapi.Serve(os.Getenv("RANKINGS_ADDR"), RankingsNetwork, src)
}

// SQLRankings is a Source over a Postgres table that already holds one row per ranked entry.
// The SQL fragments are constants chosen by the title, never request input; the board id is
// always bound as $1 (a bigint). Rank is the row's place in the ordering, ties broken by Tie.
type SQLRankings struct {
	DB *sql.DB
	// BoardsSQL returns (id text, descending bool) for every board that has rows, by id.
	BoardsSQL string
	Kind      string // rankingsapi.KindScore or KindTime
	// Name is the human name of a board; nil gives rankingsapi.NeutralName.
	Name func(id int64) string
	// From and Match select the rows of board $1; the entry's columns are named by PID,
	// Score and Updated (a timestamptz expression). Desc is a boolean expression, true when
	// a higher score ranks first. Tie orders equal scores.
	From, Match, PID, Score, Updated, Desc, Tie string
}

func (s SQLRankings) name(id int64) string {
	if s.Name != nil {
		if n := s.Name(id); n != "" {
			return n
		}
	}
	return rankingsapi.NeutralName(uint64(id))
}

func (s SQLRankings) order(desc bool) string {
	if desc {
		return rankingsapi.OrderDesc
	}
	return rankingsapi.OrderAsc
}

// Boards lists the boards that have rows.
func (s SQLRankings) Boards(ctx context.Context) ([]rankingsapi.Board, error) {
	rows, err := s.DB.QueryContext(ctx, s.BoardsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rankingsapi.Board
	for rows.Next() {
		var id string
		var desc bool
		if err := rows.Scan(&id, &desc); err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, rankingsapi.Board{ID: id, Name: s.name(n), Kind: s.Kind, Order: s.order(desc)})
	}
	return out, rows.Err()
}

// window returns the entries with place in (lo, hi] of board id, and the board's total.
func (s SQLRankings) window(ctx context.Context, id int64, lo, hi int) ([]rankingsapi.Row, error) {
	ord := fmt.Sprintf("CASE WHEN %[1]s THEN %[2]s END DESC, CASE WHEN NOT (%[1]s) THEN %[2]s END ASC, %[3]s", s.Desc, s.Score, s.Tie)
	rows, err := s.DB.QueryContext(ctx, fmt.Sprintf(`
		SELECT rn, pid, score, updated FROM (
			SELECT ROW_NUMBER() OVER (ORDER BY %s) AS rn, (%s)::text AS pid, (%s)::bigint AS score, (%s) AS updated
			FROM %s WHERE %s
		) r WHERE rn > $2 AND rn <= $3 ORDER BY rn`, ord, s.PID, s.Score, s.Updated, s.From, s.Match),
		id, lo, hi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rankingsapi.Row
	for rows.Next() {
		var r rankingsapi.Row
		var at sql.NullTime
		if err := rows.Scan(&r.Rank, &r.PID, &r.Score, &at); err != nil {
			return nil, err
		}
		r.UpdatedAt = at.Time.UTC()
		r.Display = rankingsapi.Format(s.Kind, r.Score)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Rows answers one board page, and the rows around Query.Around (a pid) when it is ranked.
func (s SQLRankings) Rows(ctx context.Context, board string, q rankingsapi.Query) (rankingsapi.Page, error) {
	id, err := strconv.ParseInt(board, 10, 64)
	if err != nil {
		return rankingsapi.Page{}, rankingsapi.ErrNoBoard
	}
	var p rankingsapi.Page
	if err := s.DB.QueryRowContext(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s`, s.From, s.Match), id).Scan(&p.Total); err != nil {
		return p, err
	}
	if p.Total == 0 {
		boards, err := s.Boards(ctx)
		if err != nil {
			return p, err
		}
		for _, b := range boards {
			if b.ID == board {
				return p, nil
			}
		}
		return p, rankingsapi.ErrNoBoard
	}
	if p.Rows, err = s.window(ctx, id, q.Offset, q.Offset+q.Limit); err != nil {
		return p, err
	}
	if q.Around != "" {
		// The player's best place on the board (one player can hold several rows).
		var rn int
		err := s.DB.QueryRowContext(ctx, fmt.Sprintf(`
			SELECT min(rn) FROM (
				SELECT ROW_NUMBER() OVER (ORDER BY CASE WHEN %[1]s THEN %[2]s END DESC, CASE WHEN NOT (%[1]s) THEN %[2]s END ASC, %[3]s) AS rn, (%[4]s)::text AS pid
				FROM %[5]s WHERE %[6]s
			) r WHERE pid = $2`, s.Desc, s.Score, s.Tie, s.PID, s.From, s.Match), id, q.Around).Scan(&sqlNullInt{&rn})
		if err != nil {
			return p, err
		}
		if rn > 0 {
			if p.Around, err = s.window(ctx, id, max(rn-rankingsapi.AroundSpan-1, 0), rn+rankingsapi.AroundSpan); err != nil {
				return p, err
			}
		}
	}
	return p, nil
}

// sqlNullInt scans a nullable integer into *int, leaving 0 for NULL.
type sqlNullInt struct{ v *int }

func (n *sqlNullInt) Scan(src any) error {
	var v sql.NullInt64
	if err := v.Scan(src); err != nil {
		return err
	}
	*n.v = int(v.Int64)
	return nil
}

// ScoresRankings reads the shared Ranking schema of the common-protocol titles (Puyo Puyo
// Tetris, Sonic & All-Stars Racing Transformed, Terraria): ranking.scores and
// ranking.categories. The category's golf_scoring flag decides the order exactly as the
// game's own ranking query does (set: higher first, unset: lower first).
func ScoresRankings(db *sql.DB, name func(int64) string) SQLRankings {
	return SQLRankings{
		DB: db,
		BoardsSQL: `SELECT s.category::text, bool_or(COALESCE(c.golf_scoring, false))
			FROM ranking.scores s LEFT JOIN ranking.categories c ON c.category = s.category
			WHERE NOT s.deleted GROUP BY s.category ORDER BY s.category`,
		Kind: rankingsapi.KindScore, Name: name,
		From:    `ranking.scores s LEFT JOIN ranking.categories c ON c.category = s.category`,
		Match:   `s.category = $1 AND NOT s.deleted`,
		PID:     `s.owner_pid`,
		Score:   `s.score`,
		Updated: `s.update_date AT TIME ZONE 'UTC'`,
		Desc:    `COALESCE(c.golf_scoring, false)`,
		Tie:     `s.update_date ASC, s.owner_pid ASC, s.unique_id ASC`,
	}
}

// LegacyRankings reads the plain rankings table of the Dr. Luigi and Miracle Cure servers
// (owner_pid, category, score, created_at in nanoseconds); the game ranks higher first.
func LegacyRankings(db *sql.DB, name func(int64) string) SQLRankings {
	return SQLRankings{
		DB:        db,
		BoardsSQL: `SELECT category::text, true FROM rankings GROUP BY category ORDER BY category`,
		Kind:      rankingsapi.KindScore, Name: name,
		From:    `rankings`,
		Match:   `category = $1`,
		PID:     `owner_pid`,
		Score:   `score`,
		Updated: `to_timestamp(created_at / 1e9)`,
		Desc:    `true`,
		Tie:     `created_at ASC, owner_pid ASC, unique_id ASC`,
	}
}

// TimeTrialRankings reads a table with one best time per (category, pid): the Mario Kart 7
// and 8 time-trial boards. table is the qualified table name and score the column or
// expression holding the time in milliseconds; lower is better.
func TimeTrialRankings(db *sql.DB, table, score string) SQLRankings {
	return SQLRankings{
		DB:        db,
		BoardsSQL: `SELECT category::text, false FROM ` + table + ` GROUP BY category ORDER BY category`,
		Kind:      rankingsapi.KindTime,
		From:      table,
		Match:     `category = $1`,
		PID:       `pid`,
		Score:     score,
		Updated:   `updated_at`,
		Desc:      `false`,
		Tie:       `updated_at ASC, pid ASC`,
	}
}

// combined serves several SQLRankings as one Source: the boards of every part, and a board's
// rows from the part that owns the id (each part's Match must be false for ids it does not own).
type combined []SQLRankings

// CombineRankings joins SQLRankings with disjoint board ids into one Source, for a title whose
// boards come from different tables or kinds (Super Mario Maker's stars and clear times).
func CombineRankings(parts ...SQLRankings) rankingsapi.Source { return combined(parts) }

func (c combined) Boards(ctx context.Context) ([]rankingsapi.Board, error) {
	var out []rankingsapi.Board
	for _, p := range c {
		b, err := p.Boards(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

func (c combined) Rows(ctx context.Context, board string, q rankingsapi.Query) (rankingsapi.Page, error) {
	for _, p := range c {
		pg, err := p.Rows(ctx, board, q)
		if errors.Is(err, rankingsapi.ErrNoBoard) {
			continue
		}
		return pg, err
	}
	return rankingsapi.Page{}, rankingsapi.ErrNoBoard
}
