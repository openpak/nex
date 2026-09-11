package mk8wiiu

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_ranking "github.com/PretendoNetwork/nex-protocols-common-go/v2/ranking"
	mk8ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/mario-kart-8"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
	"github.com/lib/pq"
)

// mk8Ranking answers Mario Kart 8's competition methods (14-16) and leaves the standard
// Ranking methods to the common implementation attached to the embedded protocol.
type mk8Ranking struct {
	*mk8ranking.Protocol
	s *Server
}

func (p *mk8Ranking) HandlePacket(packet nex.PacketInterface) {
	m := packet.RMCMessage()
	if m.IsRequest && m.ProtocolID == mk8ranking.ProtocolID {
		switch m.MethodID {
		case mk8ranking.MethodGetCompetitionRankingScore:
			dispatch(packet, mk8ranking.ProtocolID, p.s.competitionScores)
			return
		case mk8ranking.MethodUploadCompetitionRankingScore:
			dispatch(packet, mk8ranking.ProtocolID, p.s.uploadCompetitionScore)
			return
		case mk8ranking.MethodGetCompetitionInfo:
			dispatch(packet, mk8ranking.ProtocolID, p.s.competitionInfo)
			return
		}
	}
	p.Protocol.HandlePacket(packet)
}

// rankingStore is the time-trial leaderboard and common data (the player's profile blob that
// rides along with a score) in Postgres. Every Mario Kart 8 category ranks ascending: lower
// time is better.
type rankingStore struct{ db *sql.DB }

func newRankingStore(db *sql.DB) (*rankingStore, error) {
	for _, statement := range []string{
		`CREATE SCHEMA IF NOT EXISTS mk8`,
		`CREATE TABLE IF NOT EXISTS mk8.ranking_common_data (
			unique_id numeric(20) PRIMARY KEY,
			pid numeric(20) NOT NULL,
			data bytea NOT NULL,
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS mk8.ranking_scores (
			category bigint NOT NULL,
			pid numeric(20) NOT NULL,
			unique_id numeric(20) NOT NULL,
			score bigint NOT NULL,
			order_by smallint NOT NULL,
			groups bytea,
			param numeric(20) NOT NULL,
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (category, pid)
		)`,
		`CREATE INDEX IF NOT EXISTS ranking_scores_category_score ON mk8.ranking_scores (category, score)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return nil, fmt.Errorf("ranking schema: %w", err)
		}
	}
	return &rankingStore{db: db}, nil
}

func (r *rankingStore) wire(common *common_ranking.CommonProtocol, friends func(uint32) []uint32) {
	common.InsertRankingByPIDAndRankingScoreData = r.insertScore
	common.UploadCommonData = r.uploadCommonData
	common.GetCommonData = r.getCommonData
	common.GetRankingsAndCountByCategoryAndRankingOrderParam = func(category types.UInt32, param ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
		return r.rankings(uint32(category), param, nil, 0)
	}
	common.GetOwnRankingByCategoryAndRankingOrderParam = func(pid types.PID, category types.UInt32, param ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
		return r.rankings(uint32(category), param, []uint64{uint64(pid)}, 0)
	}
	common.GetFriendsRankingsAndCountByCategoryAndRankingOrderParam = func(pid types.PID, category types.UInt32, param ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
		return r.rankings(uint32(category), param, withSelf(pid, friends), 0)
	}
	common.GetNearbyRankingsAndCountByCategoryAndRankingOrderParam = func(pid types.PID, category types.UInt32, param ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
		return r.rankings(uint32(category), param, nil, uint64(pid))
	}
	common.GetNearbyFriendsRankingsAndCountByCategoryAndRankingOrderParam = func(pid types.PID, category types.UInt32, param ranking_types.RankingOrderParam) (types.List[ranking_types.RankingRankData], uint32, error) {
		return r.rankings(uint32(category), param, withSelf(pid, friends), uint64(pid))
	}
}

func withSelf(pid types.PID, friends func(uint32) []uint32) []uint64 {
	out := []uint64{uint64(pid)}
	for _, f := range friends(uint32(pid)) {
		out = append(out, uint64(f))
	}
	return out
}

func (r *rankingStore) insertScore(pid types.PID, scoreData ranking_types.RankingScoreData, uniqueID types.UInt64) error {
	_, err := r.db.Exec(
		`INSERT INTO mk8.ranking_scores (category, pid, unique_id, score, order_by, groups, param, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		 ON CONFLICT (category, pid) DO UPDATE SET
		   unique_id = EXCLUDED.unique_id, score = LEAST(mk8.ranking_scores.score, EXCLUDED.score),
		   order_by = EXCLUDED.order_by, groups = EXCLUDED.groups, param = EXCLUDED.param, updated_at = now()`,
		uint32(scoreData.Category), fmt.Sprint(uint64(pid)), fmt.Sprint(uint64(uniqueID)), uint32(scoreData.Score),
		uint8(scoreData.OrderBy), []byte(scoreData.Groups), fmt.Sprint(uint64(scoreData.Param)),
	)
	return err
}

func (r *rankingStore) uploadCommonData(pid types.PID, uniqueID types.UInt64, commonData types.Buffer) error {
	_, err := r.db.Exec(
		`INSERT INTO mk8.ranking_common_data (unique_id, pid, data, updated_at) VALUES ($1, $2, $3, now())
		 ON CONFLICT (unique_id) DO UPDATE SET pid = EXCLUDED.pid, data = EXCLUDED.data, updated_at = now()`,
		fmt.Sprint(uint64(uniqueID)), fmt.Sprint(uint64(pid)), []byte(commonData),
	)
	return err
}

func (r *rankingStore) getCommonData(uniqueID types.UInt64) (types.Buffer, error) {
	var data []byte
	err := r.db.QueryRow(`SELECT data FROM mk8.ranking_common_data WHERE unique_id = $1`, fmt.Sprint(uint64(uniqueID))).Scan(&data)
	if err != nil {
		return nil, err
	}
	return types.NewBuffer(data), nil
}

// rankings lists a category ascending by score. pids restricts to those players (friends, or
// the caller alone); nearby centres the window on that player's own rank instead of the
// requested offset. The group filter in RankingOrderParam selects by one byte of groups.
func (r *rankingStore) rankings(category uint32, param ranking_types.RankingOrderParam, pids []uint64, nearby uint64) (types.List[ranking_types.RankingRankData], uint32, error) {
	where, args := []string{"category = $1"}, []any{int64(category)}
	if pids != nil {
		strs := make([]string, len(pids))
		for i, p := range pids {
			strs[i] = fmt.Sprint(p)
		}
		args = append(args, pq.Array(strs))
		where = append(where, fmt.Sprintf("pid = ANY($%d::numeric[])", len(args)))
	}
	if uint8(param.GroupIndex) != 0xFF {
		args = append(args, int(param.GroupIndex), int(param.GroupNum))
		where = append(where, fmt.Sprintf("groups IS NOT NULL AND length(groups) > $%d AND get_byte(groups, $%d) = $%d", len(args)-1, len(args)-1, len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total uint32
	if err := r.db.QueryRow(`SELECT count(*) FROM mk8.ranking_scores WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset, length := uint32(param.Offset), uint32(param.Length)
	if length == 0 || length > 255 {
		length = 20
	}
	if nearby != 0 {
		var rank uint32
		err := r.db.QueryRow(`SELECT count(*) FROM mk8.ranking_scores s WHERE `+cond+
			` AND s.score < (SELECT score FROM mk8.ranking_scores WHERE category = $1 AND pid = $`+fmt.Sprint(len(args)+1)+`)`,
			append(args, fmt.Sprint(nearby))...).Scan(&rank)
		if err != nil {
			return nil, total, nil // the player has no score: nothing is "nearby"
		}
		if rank > length/2 {
			offset = rank - length/2
		} else {
			offset = 0
		}
	}
	rows, err := r.db.Query(`SELECT s.pid, s.unique_id, s.score, s.groups, s.param, s.updated_at, coalesce(c.data, '')
		FROM mk8.ranking_scores s LEFT JOIN mk8.ranking_common_data c ON c.unique_id = s.unique_id
		WHERE `+cond+fmt.Sprintf(` ORDER BY s.score ASC, s.updated_at ASC OFFSET %d LIMIT %d`, offset, length), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := types.NewList[ranking_types.RankingRankData]()
	rank := offset
	for rows.Next() {
		var pid, uniqueID, paramStr string
		var score int64
		var groups, common []byte
		var updated time.Time
		if err := rows.Scan(&pid, &uniqueID, &score, &groups, &paramStr, &updated, &common); err != nil {
			return nil, 0, err
		}
		rank++
		d := ranking_types.NewRankingRankData()
		var p, u, pr uint64
		fmt.Sscan(pid, &p)
		fmt.Sscan(uniqueID, &u)
		fmt.Sscan(paramStr, &pr)
		d.PrincipalID = types.NewPID(p)
		d.UniqueID = types.NewUInt64(u)
		d.Order = types.NewUInt32(rank)
		d.Category = types.NewUInt32(category)
		d.Score = types.NewUInt32(uint32(score))
		d.Groups = types.NewBuffer(groups)
		d.Param = types.NewUInt64(pr)
		d.CommonData = types.NewBuffer(common)
		d.UpdateTime = fromTime(updated)
		list = append(list, d)
	}
	return list, total, nil
}

// --- competitions (tournament leaderboards), methods 14-16 ---

// teamScores is the four-value block the game shows for a team tournament: team 0 score,
// team 1 score, team 0 participants, team 1 participants. Pretendo's server reported the sum of
// scores minus the participant count for each team's score; kept for wire compatibility.
func (s *Server) teamScores(tournamentID uint32, seasonID *uint32) []uint32 {
	out := make([]uint32, 4)
	q := `SELECT team_id, coalesce(sum(score),0), count(*) FROM mk8.tournament_scores WHERE tournament_id = $1`
	args := []any{int64(tournamentID)}
	if seasonID != nil {
		q += ` AND season_id = $2`
		args = append(args, int64(*seasonID))
	}
	rows, err := s.db.Query(q+` GROUP BY team_id`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var team int
		var sum, n int64
		if rows.Scan(&team, &sum, &n) == nil && (team == 0 || team == 1) {
			out[team] = uint32(sum - n)
			out[2+team] = uint32(n)
		}
	}
	return out
}

func (s *Server) competitionScores(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error {
	var id uint32
	var rng resultRange
	if err := readStructure(in, func(in *nex.ByteStreamIn) error {
		var e error
		if id, e = readU32(in); e != nil {
			return e
		}
		rng, e = readResultRange(in)
		return e
	}); err != nil {
		return invalid("%v", err)
	}
	if rng.Length > 5 {
		return invalid("at most five seasons")
	}
	var attributes pq.Int64Array
	var current int64
	err := s.db.QueryRow(`SELECT attributes, season_id FROM mk8.tournaments WHERE id = $1`, int64(id)).Scan(&attributes, &current)
	if err == sql.ErrNoRows {
		return nex.NewError(nex.ResultCodes.Ranking.InvalidArgument, "no such tournament")
	}
	if err != nil {
		return storeError(err)
	}
	team := len(attributes) > 4 && attributes[4] == 2
	first := current - int64(rng.Length)
	if first < 1 {
		first = 1
	}
	var infos []competitionScoreInfo
	for season := first; season <= current; season++ {
		sid := uint32(season)
		info := competitionScoreInfo{SeasonID: sid, TeamScores: make([]uint32, 4)}
		rows, err := s.db.Query(`SELECT pid, score, last_update, team_id, metadata FROM mk8.tournament_scores
			WHERE tournament_id = $1 AND season_id = $2 ORDER BY score DESC LIMIT 20`, int64(id), season)
		if err != nil {
			return storeError(err)
		}
		for i := uint32(1); rows.Next(); i++ {
			var pid string
			var score int64
			var updated time.Time
			var teamID int
			var meta []byte
			if err := rows.Scan(&pid, &score, &updated, &teamID, &meta); err != nil {
				rows.Close()
				return storeError(err)
			}
			var p uint64
			fmt.Sscan(pid, &p)
			info.Scores = append(info.Scores, competitionScore{Rank: i, PID: p, Score: uint32(score),
				LastUpdate: uint64(fromTime(updated)), TeamID: uint8(teamID), Metadata: meta})
		}
		rows.Close()
		s.db.QueryRow(`SELECT count(*) FROM mk8.tournament_scores WHERE tournament_id = $1 AND season_id = $2`, int64(id), season).Scan(&info.NumParticipants)
		if team {
			info.TeamScores = s.teamScores(id, &sid)
		}
		infos = append(infos, info)
	}
	out.WriteUInt32LE(uint32(len(infos)))
	for _, i := range infos {
		i.write(out)
	}
	return nil
}

func (s *Server) uploadCompetitionScore(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error {
	p, err := readUploadScoreParam(in)
	if err != nil {
		return invalid("%v", err)
	}
	if len(p.Metadata) > 0x100 {
		return invalid("metadata too long")
	}
	var current int64
	err = s.db.QueryRow(`SELECT season_id FROM mk8.tournaments WHERE id = $1`, int64(p.ID)).Scan(&current)
	if err == sql.ErrNoRows {
		return nex.NewError(nex.ResultCodes.Ranking.InvalidArgument, "no such tournament")
	}
	if err != nil {
		return storeError(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return storeError(err)
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO mk8.tournament_scores (tournament_id, season_id, pid, score, team_id, team_score, metadata, last_update)
		VALUES ($1,$2,$3,$4,$5,$6,$7,now())
		ON CONFLICT (tournament_id, season_id, pid) DO UPDATE SET score = EXCLUDED.score, team_id = EXCLUDED.team_id,
		  team_score = EXCLUDED.team_score, metadata = EXCLUDED.metadata, last_update = now()`,
		int64(p.ID), int64(p.SeasonID), fmt.Sprint(uint64(conn.PID())), int64(p.Score), int(p.TeamID), int64(p.TeamScore), p.Metadata)
	if err != nil {
		return storeError(err)
	}
	if n, _ := res.RowsAffected(); n == 1 && p.IsFirstUpload {
		if _, err := tx.Exec(`UPDATE mk8.tournaments SET total_participants = (SELECT count(DISTINCT pid) FROM mk8.tournament_scores WHERE tournament_id = $1) WHERE id = $1`, int64(p.ID)); err != nil {
			return storeError(err)
		}
	}
	if int64(p.SeasonID) > current {
		if _, err := tx.Exec(`UPDATE mk8.tournaments SET season_id = $2 WHERE id = $1`, int64(p.ID), int64(p.SeasonID)); err != nil {
			return storeError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return storeError(err)
	}
	out.WriteBool(true)
	return nil
}

func (s *Server) competitionInfo(in *nex.ByteStreamIn, conn *nex.PRUDPConnection, out *nex.ByteStreamOut) *nex.Error {
	var rng resultRange
	if err := readStructure(in, func(in *nex.ByteStreamIn) error {
		if _, e := readU8(in); e != nil { // rank order; the game only asks for popularity
			return e
		}
		var e error
		rng, e = readResultRange(in)
		return e
	}); err != nil {
		return invalid("%v", err)
	}
	if rng.Length > 100 {
		return invalid("at most 100 tournaments")
	}
	limit := rng.Length
	if limit == 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, attributes, total_participants FROM mk8.tournaments
		WHERE attributes[1] = 1 AND attributes[13] <> 2 AND attributes[14] <> 2
		ORDER BY total_participants DESC, id OFFSET $1 LIMIT $2`, int64(rng.Offset), int64(limit))
	if err != nil {
		return storeError(err)
	}
	defer rows.Close()
	var infos []competitionInfo
	for rows.Next() {
		var id, participants int64
		var attributes pq.Int64Array
		if err := rows.Scan(&id, &attributes, &participants); err != nil {
			return storeError(err)
		}
		info := competitionInfo{ID: uint32(id), NumParticipants: uint32(participants), TeamScores: make([]uint32, 4)}
		if len(attributes) > 4 && attributes[4] == 2 {
			info.TeamScores = s.teamScores(uint32(id), nil)
		}
		infos = append(infos, info)
	}
	out.WriteUInt32LE(uint32(len(infos)))
	for _, i := range infos {
		i.write(out)
	}
	return nil
}
