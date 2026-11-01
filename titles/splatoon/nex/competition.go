package nex

// Ranking (Splatoon): the Splatfest competition ranking.
//
// The game uploads one record per finished Splatfest match (method 0x12) and reads a
// festival's per-team totals and score list back (method 0x10). This is the "Splatfest
// results" screen's source, per kinnay/NintendoClients#163 and the wiki page. Judd
// (the BOSS web-post receiver) stores a second, richer copy of each match; SplatNet
// shows both.
//
// Storage is Postgres, schema ranking_splatoon, two tables:
//
//	results      one row per upload (the log; the team win tally sums it)
//	user_scores  one row per (player, festival): their best score and their team
//
// Tally rules (not yet console-verified, see docs/splatfest.md):
//
//	votes[team] = players whose user_scores row names that team
//	wins[team]  = sum of TeamScore over that team's uploads

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/splatoon"
	nexhost "github.com/openpak/nex"
	"github.com/openpak/nex/titles/splatoon/globals"
	emit "github.com/openpak/rewards-emit"
)

const (
	// teams in a Splatfest: 0 = Alpha, 1 = Bravo.
	festTeams = 2
	// defaultScoreListLength caps a score list when the request names no length.
	defaultScoreListLength = 100
	// maxScoreListLength is the most one reply carries.
	maxScoreListLength = 500
)

// festStore is the Postgres side of the competition ranking.
type festStore struct{ db *sql.DB }

func (s festStore) setup() error {
	for _, q := range []string{
		`CREATE SCHEMA IF NOT EXISTS ranking_splatoon`,
		`CREATE TABLE IF NOT EXISTS ranking_splatoon.results (
			upload_id bigserial PRIMARY KEY,
			uploader_pid bigint NOT NULL,
			splatfest_id bigint NOT NULL,
			team_id smallint NOT NULL,
			score bigint NOT NULL,
			team_score bigint NOT NULL,
			is_first_upload boolean NOT NULL,
			app_data bytea NOT NULL DEFAULT ''::bytea,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS results_fest_idx ON ranking_splatoon.results (splatfest_id, team_id)`,
		`CREATE TABLE IF NOT EXISTS ranking_splatoon.user_scores (
			uploader_pid bigint NOT NULL,
			splatfest_id bigint NOT NULL,
			team_id smallint NOT NULL,
			score bigint NOT NULL,
			team_score bigint NOT NULL DEFAULT 0,
			app_data bytea NOT NULL DEFAULT ''::bytea,
			last_updated timestamptz NOT NULL DEFAULT now(),
			created_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (uploader_pid, splatfest_id)
		)`,
		`CREATE INDEX IF NOT EXISTS user_scores_fest_idx ON ranking_splatoon.user_scores (splatfest_id, score DESC)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// record stores one uploaded match: the log row, and the player's festival row (best
// score kept, team and latest team score updated).
func (s festStore) record(pid uint64, p CompetitionRankingUploadScoreParam) error {
	if p.TeamID >= festTeams {
		return fmt.Errorf("team id %d is not a Splatfest team", p.TeamID)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var resultID int64
	if err = tx.QueryRow(`INSERT INTO ranking_splatoon.results
		(uploader_pid, splatfest_id, team_id, score, team_score, is_first_upload, app_data)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING upload_id`,
		int64(pid), int64(p.FestID), int16(p.TeamID), int64(p.Score), int64(p.TeamScore), bool(p.IsFirstUpload), []byte(p.AppData)).Scan(&resultID); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO ranking_splatoon.user_scores
		(uploader_pid, splatfest_id, team_id, score, team_score, app_data)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (uploader_pid, splatfest_id) DO UPDATE SET
			team_id = EXCLUDED.team_id,
			score = GREATEST(ranking_splatoon.user_scores.score, EXCLUDED.score),
			team_score = EXCLUDED.team_score,
			app_data = EXCLUDED.app_data,
			last_updated = now()`,
		int64(pid), int64(p.FestID), int16(p.TeamID), int64(p.Score), int64(p.TeamScore), []byte(p.AppData)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rewardsFestMatch(pid, uint32(p.FestID), resultID)
	return nil
}

// rewardsFestMatch tells rewards a finished Splatfest match was stored: one Splatfest joined
// (distinct per festival) and one completed Splatoon match. The result row id makes the event
// id unique per upload, so a retried upload never counts twice. Only Splatfest matches reach
// the server; ordinary Turf War and Ranked matches leave no trace here.
func rewardsFestMatch(pid uint64, fest uint32, resultID int64) {
	rw := nexhost.Rewards()
	who := nexhost.PlayerOf("wiiu", pid)
	rw.Event(emit.Event{ID: fmt.Sprintf("splatoon:fest:%d:%d:joined", fest, pid), Metric: "splatfests_joined", Player: who, Keys: []string{fmt.Sprint(fest)}})
	rw.Event(emit.Event{ID: fmt.Sprintf("splatoon:fest-match:%d", resultID), Metric: "splatoon_matches_completed.s1", Player: who})
}

// Tally is a festival's per-team totals.
type Tally struct {
	FestID  uint32
	Votes   [festTeams]uint32
	Wins    [festTeams]uint32
	Players uint32
}

func (s festStore) tally(festID uint32) (Tally, error) {
	t := Tally{FestID: festID}
	rows, err := s.db.Query(`SELECT team_id, count(*) FROM ranking_splatoon.user_scores
		WHERE splatfest_id = $1 AND team_id < $2 GROUP BY team_id`, int64(festID), festTeams)
	if err != nil {
		return t, err
	}
	for rows.Next() {
		var team int16
		var n int64
		if err = rows.Scan(&team, &n); err != nil {
			rows.Close()
			return t, err
		}
		t.Votes[team] = clamp32(n)
		t.Players += clamp32(n)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return t, err
	}
	rows, err = s.db.Query(`SELECT team_id, COALESCE(sum(team_score), 0) FROM ranking_splatoon.results
		WHERE splatfest_id = $1 AND team_id < $2 GROUP BY team_id`, int64(festID), festTeams)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	for rows.Next() {
		var team int16
		var n int64
		if err = rows.Scan(&team, &n); err != nil {
			return t, err
		}
		t.Wins[team] = clamp32(n)
	}
	return t, rows.Err()
}

func clamp32(n int64) uint32 {
	switch {
	case n < 0:
		return 0
	case n > 0xFFFFFFFF:
		return 0xFFFFFFFF
	}
	return uint32(n)
}

// info builds one festival's reply entry. offset/length page the score list.
func (s festStore) info(festID uint32, offset, length uint32) (CompetitionRankingScoreInfo, error) {
	info := NewCompetitionRankingScoreInfo()
	info.FestID = types.NewUInt32(festID)

	t, err := s.tally(festID)
	if err != nil {
		return info, err
	}
	for team := 0; team < festTeams; team++ {
		info.TeamWins = append(info.TeamWins, types.NewUInt32(t.Wins[team]))
		info.TeamVotes = append(info.TeamVotes, types.NewUInt32(t.Votes[team]))
	}

	if length == 0 || length > maxScoreListLength {
		length = defaultScoreListLength
		if length > maxScoreListLength {
			length = maxScoreListLength
		}
	}
	rows, err := s.db.Query(`SELECT uploader_pid, team_id, score, last_updated, app_data
		FROM ranking_splatoon.user_scores WHERE splatfest_id = $1
		ORDER BY score DESC, last_updated ASC, uploader_pid ASC
		LIMIT $2 OFFSET $3`, int64(festID), int64(length), int64(offset))
	if err != nil {
		return info, err
	}
	defer rows.Close()
	position := offset
	for rows.Next() {
		var pid, score int64
		var team int16
		var updated sql.NullTime
		var appData []byte
		if err = rows.Scan(&pid, &team, &score, &updated, &appData); err != nil {
			return info, err
		}
		position++
		d := NewCompetitionRankingScoreData()
		d.Unknown1 = types.NewUInt32(position)
		d.PID = types.NewPID(uint64(pid))
		d.Score = types.NewUInt32(clamp32(score))
		if updated.Valid {
			d.Modified = d.Modified.FromTimestamp(updated.Time)
		}
		d.Unknown2 = types.NewUInt8(uint8(team))
		d.AppData = types.NewQBuffer(appData)
		info.ScoreData = append(info.ScoreData, d)
	}
	return info, rows.Err()
}

// festIDs is the festival ids a GetCompetitionRankingScore request names. The wiki puts
// them in FestivalIDs; upstream's draft read ResultRange.Offset instead, so a request
// with an empty list and a non-zero offset is read that way too rather than answered
// empty.
func festIDs(p CompetitionRankingGetParam) (ids []uint32, paged bool) {
	for _, id := range p.FestivalIDs {
		ids = append(ids, uint32(id))
	}
	if len(ids) == 0 && p.ResultRange.Offset != 0 && p.ResultRange.Offset != 0xFFFFFFFF {
		return []uint32{uint32(p.ResultRange.Offset)}, false
	}
	return ids, true
}

// getCompetitionRankingScore answers method 0x10.
func (s festStore) getCompetitionRankingScore(err error, packet nex.PacketInterface, callID uint32, _ []byte) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}
	endpoint := packet.Sender().Endpoint()
	params := NewCompetitionRankingGetParam()
	in := nex.NewByteStreamIn(packet.RMCMessage().Parameters, endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	if e := params.ExtractFrom(in); e != nil {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}

	ids, paged := festIDs(params)
	if len(ids) > 8 {
		ids = ids[:8]
	}
	var offset, length uint32
	if paged {
		offset, length = uint32(params.ResultRange.Offset), uint32(params.ResultRange.Length)
		if offset == 0xFFFFFFFF {
			offset = 0
		}
	}
	reply := types.NewList[CompetitionRankingScoreInfo]()
	for _, id := range ids {
		info, e := s.info(id, offset, length)
		if e != nil {
			globals.Logger.Errorf("Ranking: competition score read failed: %v", e)
			return nil, nex.NewError(nex.ResultCodes.Core.Exception, "change_error")
		}
		reply = append(reply, info)
	}

	out := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	reply.WriteTo(out)
	msg := nex.NewRMCSuccess(endpoint, out.Bytes())
	msg.ProtocolID = ranking.ProtocolID
	msg.MethodID = ranking.MethodGetCompetitionRankingScore
	msg.CallID = callID
	return msg, nil
}

// uploadCompetitionRankingScore answers method 0x12.
func (s festStore) uploadCompetitionRankingScore(err error, packet nex.PacketInterface, callID uint32, _ []byte) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}
	endpoint := packet.Sender().Endpoint()
	params := NewCompetitionRankingUploadScoreParam()
	in := nex.NewByteStreamIn(packet.RMCMessage().Parameters, endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	if e := params.ExtractFrom(in); e != nil {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}
	if len(params.AppData) > 4096 {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}
	pid := uint64(packet.Sender().PID())
	if e := s.record(pid, params); e != nil {
		if errors.Is(e, sql.ErrConnDone) {
			return nil, nex.NewError(nex.ResultCodes.Core.Exception, "change_error")
		}
		globals.Logger.Errorf("Ranking: competition score write failed: %v", e)
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "change_error")
	}

	out := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	types.NewBool(true).WriteTo(out)
	msg := nex.NewRMCSuccess(endpoint, out.Bytes())
	msg.ProtocolID = ranking.ProtocolID
	msg.MethodID = ranking.MethodUploadCompetitionRankingScore
	msg.CallID = callID
	return msg, nil
}

// wire installs the two handlers on the Splatoon ranking protocol.
func (s festStore) wire(p *ranking.Protocol) {
	p.GetCompetitionRankingScore = s.getCompetitionRankingScore
	p.UploadCompetitionRankingScore = s.uploadCompetitionRankingScore
}
