package database

import (
	"database/sql"
	"os"

	_ "github.com/lib/pq"

	"github.com/openpak/nex/titles/pikmin-3/board"
	"github.com/openpak/nex/titles/pikmin-3/globals"
)

var Postgres *sql.DB

// Board is the leaderboard store over Postgres.
var Board *board.Store

func ConnectPostgres() {
	var err error

	Postgres, err = sql.Open("postgres", os.Getenv("PN_PIKMIN3_POSTGRES_URI"))
	if err != nil {
		globals.Logger.Critical(err.Error())
	}

	globals.Logger.Success("Connected to Postgres!")

	Board = &board.Store{DB: Postgres, Friends: friendPIDs}
	if err := Board.Migrate(); err != nil {
		globals.Logger.Critical(err.Error())
		return
	}
	globals.Logger.Success("Postgres tables created")
}
