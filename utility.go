package lm3

// Utility (0x6E) settings.
//
// The game calls GetIntegerSettings right after SecureConnection::Register, and a reply the
// client cannot parse drops the connection about a second later. nex-protocols-common-go reads
// the request correctly; the map's *content* is the operator's, and we do not know it — no
// capture of this title's settings exists. So the honest answer is a correctly shaped empty
// map for every index: right on the wire, possibly incomplete in content.
//
// ponytail: empty maps, no env-configurable override. Add one (see the sibling
// mario-party-superstars) if a capture ever turns up a real setting this title needs.

import (
	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
)

func integerSettings(_ *common_globals.UtilityManager, _ types.PID, _ types.UInt32) (types.Map[types.UInt16, types.UInt32], *nex.Error) {
	return types.NewMap[types.UInt16, types.UInt32](), nil
}

func stringSettings(_ *common_globals.UtilityManager, _ types.PID, _ types.UInt32) (types.Map[types.UInt16, types.String], *nex.Error) {
	return types.NewMap[types.UInt16, types.String](), nil
}
