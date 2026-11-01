package nex_datastore_super_mario_maker

import (
	"fmt"

	"github.com/PretendoNetwork/nex-go/v2"
	datastore_super_mario_maker "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/super-mario-maker"
	datastore_super_mario_maker_types "github.com/PretendoNetwork/nex-protocols-go/v2/datastore/super-mario-maker/types"
	nexhost "github.com/openpak/nex"
	datastore_db "github.com/openpak/nex/titles/super-mario-maker/database/datastore"
	datastore_smm_db "github.com/openpak/nex/titles/super-mario-maker/database/datastore/super-mario-maker"
	"github.com/openpak/nex/titles/super-mario-maker/globals"
	emit "github.com/openpak/rewards-emit"
)

func UploadCourseRecord(err error, packet nex.PacketInterface, callID uint32, param datastore_super_mario_maker_types.DataStoreUploadCourseRecordParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		globals.Logger.Error(err.Error())
		return nil, nex.NewError(nex.ResultCodes.DataStore.Unknown, err.Error())
	}

	client := packet.Sender()

	nexError := datastore_smm_db.InsertOrUpdateCourseRecord(param.DataID, param.Slot, client.PID(), param.Score)
	if nexError != nil {
		return nil, nexError
	}

	// A stored course record means the player cleared the course: one course played. Clearing
	// your own course (required before an upload) does not count.
	if owner, e := datastore_db.GetObjectOwnerByDataID(param.DataID); e == nil && uint64(owner) != uint64(client.PID()) {
		nexhost.Rewards().Event(emit.Event{
			ID:     fmt.Sprintf("smm:clear:%d:%d", uint64(param.DataID), uint64(client.PID())),
			Metric: "maker_courses_played", Player: nexhost.PlayerOf("wiiu", uint64(client.PID())),
			Keys: []string{fmt.Sprint(uint64(param.DataID))},
		})
	}

	rmcResponse := nex.NewRMCSuccess(globals.SecureEndpoint, nil)
	rmcResponse.ProtocolID = datastore_super_mario_maker.ProtocolID
	rmcResponse.MethodID = datastore_super_mario_maker.MethodUploadCourseRecord
	rmcResponse.CallID = callID

	return rmcResponse, nil
}
