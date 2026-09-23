package nexhost

// Bans: an account banned in the OpenPak admin is locked out of every NEX title
// (website/docs/ban-lookup.md is the contract).
//
// Each shape asks the service that already vouches for the player at login:
//
//   - Switch titles (Register shape, this Host) prove every console through nx-baas's
//     /internal/switch/identity, and nx-baas answers 403 {"error":"account_banned"} (or
//     "account_deleted") for a locked-out account. That becomes errAccountBanned in account.go.
//   - Wii U and 3DS titles (Run shape, Pretendo-derived) ask nn-account for the NEX password on
//     every login, and nn-account refuses a banned owner there. NEXPasswordResult turns that
//     refusal into the native banned result instead of "invalid username".
//
// Either way the refusal is RendezVous::AccountDisabled (0x00030067), the NEX result code for
// a disabled account, and WatchBans drops live connections whose account became banned.

import (
	"errors"
	"log"
	"strings"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// banSweepEvery is how often WatchBans re-asks about connected players. nx-baas and
// nn-account cache an "active" answer for at most 30 s, so a ban reaches a live connection
// within a minute.
const banSweepEvery = 30 * time.Second

// errAccountBanned is nx-baas refusing the account itself: banned, or being deleted.
var errAccountBanned = errors.New("account banned")

// bannedResult is the NEX error for a locked-out account.
func bannedResult() *nex.Error {
	return nex.NewError(nex.ResultCodes.RendezVous.AccountDisabled, "account banned")
}

// isBanned reports whether a NEX error is the account-disabled result. nex.NewError sets the
// error bit, so the comparison ignores it.
func isBanned(e *nex.Error) bool {
	const errorBit = 0x80000000
	return e != nil && e.ResultCode|errorBit == nex.ResultCodes.RendezVous.AccountDisabled|errorBit
}

// NEXPasswordResult is the NEX result a Wii U/3DS title answers when nn-account's
// GetNEXPassword fails. nn-account refuses a banned (or deleting) owner with InvalidArgument
// "Account is banned or deleted"; that becomes RendezVous::AccountDisabled so the console
// shows a disabled account. Everything else stays RendezVous::InvalidUsername, the upstream
// answer.
func NEXPasswordResult(err error) uint32 {
	if s, ok := status.FromError(err); ok && s.Code() == codes.InvalidArgument &&
		strings.Contains(strings.ToLower(s.Message()), "banned") {
		return nex.ResultCodes.RendezVous.AccountDisabled
	}
	return nex.ResultCodes.RendezVous.InvalidUsername
}

// WatchBans drops live connections on ep whose account is now locked out. Every
// banSweepEvery it asks ep.AccountDetailsByPID -- the same function login uses, so whatever
// refuses a banned login here also ends a banned session -- for every connected user PID, and
// cleans up those answered RendezVous::AccountDisabled. Any other failure keeps the session:
// mid-session, only a positive "banned" ends it (contract rule 4). The console loses its
// connection, and its next login is refused with the same code.
//
// Run-shaped titles call it once after creating their secure endpoint; the Host starts it
// itself. It never returns.
func WatchBans(title string, ep *nex.PRUDPEndPoint) {
	for {
		time.Sleep(banSweepEvery)
		sweepBans(title, ep)
	}
}

func sweepBans(title string, ep *nex.PRUDPEndPoint) {
	lookup := ep.AccountDetailsByPID
	if lookup == nil {
		return
	}
	var connected []*nex.PRUDPConnection
	ep.Connections.Each(func(_ string, c *nex.PRUDPConnection) bool {
		if uint64(c.PID()) != 0 { // 0 = not logged in yet; the server accounts always resolve
			connected = append(connected, c)
		}
		return false
	})
	checked := map[uint64]bool{} // one lookup per PID per sweep
	for _, c := range connected {
		pid := uint64(c.PID())
		banned, seen := checked[pid]
		if !seen {
			_, nexErr := lookup(types.NewPID(pid))
			banned = isBanned(nexErr)
			checked[pid] = banned
		}
		if banned {
			log.Printf("[%s] pid %d is banned, dropping its connection", title, pid)
			ep.CleanupConnection(c)
		}
	}
}
