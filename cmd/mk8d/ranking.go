package main

// The Ranking (0x70) backend.
//
// MK8D uses this protocol for two unrelated things, and only one of them is a leaderboard:
//
//   - UploadScore (method 1) posts a time-trial score for a category.
//   - UploadCommonData (4) / GetCommonData (6) store and fetch an opaque per-player blob
//     that other consoles read to draw a racer's name and Mii. Nothing on the server
//     interprets it, so it is stored as bytes.
//
// Two tables, no ORM, no cache. The scores table exists so uploads are not thrown away
// before there is anything to read them back with.

import (
	"database/sql"
	"fmt"

	"github.com/PretendoNetwork/nex-go/v2/types"
	ranking_types "github.com/PretendoNetwork/nex-protocols-go/v2/ranking/types"
)

type rankingStore struct{ db *sql.DB }

// newRankingStore creates the schema on first run, the way the common protocols create
// theirs. A NEX unique ID and a PID are both up to 64 bits unsigned, which is numeric(20)
// in Postgres -- bigint would wrap.
func newRankingStore(db *sql.DB) (*rankingStore, error) {
	for _, statement := range []string{
		`CREATE SCHEMA IF NOT EXISTS ranking`,
		`CREATE TABLE IF NOT EXISTS ranking.common_data (
			unique_id numeric(20) PRIMARY KEY,
			pid numeric(20) NOT NULL,
			data bytea NOT NULL,
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		// One row per player per category: a second upload of the same time trial
		// replaces the first, which is what "update mode" means for every mode MK8D
		// has been seen to use.
		// ponytail: last write wins. If a capture shows MK8D relying on
		// UpdateMode "only if better", compare on score here.
		`CREATE TABLE IF NOT EXISTS ranking.scores (
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
	} {
		if _, err := db.Exec(statement); err != nil {
			return nil, fmt.Errorf("ranking schema: %w", err)
		}
	}
	return &rankingStore{db: db}, nil
}

func (s *rankingStore) insertScore(pid types.PID, scoreData ranking_types.RankingScoreData, uniqueID types.UInt64) error {
	_, err := s.db.Exec(
		`INSERT INTO ranking.scores (category, pid, unique_id, score, order_by, groups, param, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		 ON CONFLICT (category, pid) DO UPDATE SET
		   unique_id = EXCLUDED.unique_id, score = EXCLUDED.score, order_by = EXCLUDED.order_by,
		   groups = EXCLUDED.groups, param = EXCLUDED.param, updated_at = now()`,
		uint32(scoreData.Category), uint64(pid), uint64(uniqueID), uint32(scoreData.Score),
		uint8(scoreData.OrderBy), []byte(scoreData.Groups), uint64(scoreData.Param),
	)
	return err
}

func (s *rankingStore) uploadCommonData(pid types.PID, uniqueID types.UInt64, commonData types.Buffer) error {
	_, err := s.db.Exec(
		`INSERT INTO ranking.common_data (unique_id, pid, data, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (unique_id) DO UPDATE SET pid = EXCLUDED.pid, data = EXCLUDED.data, updated_at = now()`,
		uint64(uniqueID), uint64(pid), []byte(commonData),
	)
	return err
}

// getCommonData returns the blob for a unique ID. A miss is an error on purpose: the common
// handler turns it into Ranking::NotFound, which is what a console asking about a player who
// has never uploaded should hear.
func (s *rankingStore) getCommonData(uniqueID types.UInt64) (types.Buffer, error) {
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM ranking.common_data WHERE unique_id = $1`, uint64(uniqueID)).Scan(&data)
	if err != nil {
		return nil, err
	}
	return types.NewBuffer(data), nil
}
