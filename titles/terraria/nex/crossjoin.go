package nex

import (
	"sync/atomic"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	matchmakeextension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	"github.com/openpak/nex/titles/terraria/globals"
)

// CrossJoinsRefused counts Wii U joins into another console's room since start (prds#145): cross-play listing is on, the relay
// that would carry the game is not built yet, so each attempt is answered and counted to keep the gap visible.
var CrossJoinsRefused atomic.Int64

// crossJoinError answers a join into a shadow gathering with RendezVous SessionClosed (participation closed), the closest
// result the game already shows as "could not join"; nil for every other gathering.
func crossJoinError(gid types.UInt32) *nex.Error {
	if globals.IsCrossRoom == nil || !globals.IsCrossRoom(uint32(gid)) {
		return nil
	}
	n := CrossJoinsRefused.Add(1)
	globals.Logger.Warningf("cross-play join not yet supported (relay pending): gathering %d, refused %d since start", uint32(gid), n)
	return nex.NewError(nex.ResultCodes.RendezVous.SessionClosed, "cross-play join not yet supported (relay pending)")
}

// refuseCrossJoins wraps the join handlers the common protocol installed.
func refuseCrossJoins(p *matchmakeextension.Protocol) {
	if join := p.JoinMatchmakeSession; join != nil {
		p.JoinMatchmakeSession = func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32, msg types.String) (*nex.RMCMessage, *nex.Error) {
			if e := crossJoinError(gid); err == nil && e != nil {
				return nil, e
			}
			return join(err, packet, callID, gid, msg)
		}
	}
	if join := p.JoinMatchmakeSessionEx; join != nil {
		p.JoinMatchmakeSessionEx = func(err error, packet nex.PacketInterface, callID uint32, gid types.UInt32, msg types.String, dontCare types.Bool, count types.UInt16) (*nex.RMCMessage, *nex.Error) {
			if e := crossJoinError(gid); err == nil && e != nil {
				return nil, e
			}
			return join(err, packet, callID, gid, msg, dontCare, count)
		}
	}
}
