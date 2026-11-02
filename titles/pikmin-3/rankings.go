package pikmin3

import (
	"context"
	"strconv"
	"time"

	"github.com/openpak/nex/internal/rankingsapi"
	"github.com/openpak/nex/titles/pikmin-3/board"
)

// Rankings is the rankings contract over the leaderboard store: one board per category,
// ranked by board.Rank exactly as the game's range query is (the category's order comes
// from its newest upload, ties go to the older score, then the lower pid). The category to
// stage map is unknown, so boards carry neutral names.
type Rankings struct{ Store *board.Store }

// Boards lists every category that has a score.
func (r Rankings) Boards(ctx context.Context) ([]rankingsapi.Board, error) {
	cats, err := r.Store.Categories()
	if err != nil {
		return nil, err
	}
	out := make([]rankingsapi.Board, 0, len(cats))
	for _, c := range cats {
		_, orderBy, err := r.Store.Category(c)
		if err != nil {
			return nil, err
		}
		out = append(out, rankingsapi.Board{ID: strconv.FormatUint(uint64(c), 10), Name: rankingsapi.NeutralName(uint64(c)),
			Kind: rankingsapi.KindScore, Order: order(orderBy)})
	}
	return out, nil
}

func order(orderBy uint8) string {
	if orderBy == board.Ascending {
		return rankingsapi.OrderAsc
	}
	return rankingsapi.OrderDesc
}

// Rows answers one category.
func (r Rankings) Rows(ctx context.Context, id string, q rankingsapi.Query) (rankingsapi.Page, error) {
	n, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return rankingsapi.Page{}, rankingsapi.ErrNoBoard
	}
	entries, orderBy, err := r.Store.Category(uint32(n))
	if err != nil {
		return rankingsapi.Page{}, err
	}
	if len(entries) == 0 {
		return rankingsapi.Page{}, rankingsapi.ErrNoBoard
	}
	ranked := board.Rank(entries, orderBy, board.Calc123)
	all := make([]rankingsapi.Row, len(ranked))
	for i, e := range ranked {
		all[i] = rankingsapi.Row{PID: strconv.FormatUint(uint64(e.PID), 10), Score: int64(e.Score),
			UpdatedAt: time.Unix(0, e.UpdatedAt).UTC()}
	}
	return rankingsapi.Window(rankingsapi.KindScore, all, q), nil
}
