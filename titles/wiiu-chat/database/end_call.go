package database

import (
	"github.com/PretendoNetwork/nex-go/v2/types"
	"github.com/openpak/nex/titles/wiiu-chat/globals"
)

func EndCall(caller types.PID) {
	_, err := Postgres.Exec(`DELETE FROM ongoingcalls WHERE caller_pid = $1;`, caller)
	if err != nil {
		globals.Logger.Critical(err.Error())
		return
	}
}
