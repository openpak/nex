package smb35

// The Eagle handoff.
//
// Super Mario Bros. 35 does not play peer to peer. NEX only forms the lobby; the match
// itself runs over Eagle, a relay Nintendo used for exactly four titles (Tetris 99,
// Super Mario Bros. 35, PAC-MAN 99, F-Zero 99). When a player joins a lobby the NEX server
// pushes one notification telling them where the relay is and handing them a token to
// present to it. Without that notification the game sits in a full lobby forever: it is
// waiting for an address, and no error is ever shown.
//
// The relay itself is not in this repository and does not exist in OpenPak yet. It is
// shared by four titles, so it belongs in a repository of its own rather than here; what
// this file owns is the half the NEX server is responsible for — the address, the token,
// and the notification that carries them.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/openpak/nex/redact"
	"log"
	"strconv"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	matchmake_extension_database "github.com/PretendoNetwork/nex-protocols-common-go/v2/matchmake-extension/database"
	notifications_constants "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/constants"
	notifications_types "github.com/PretendoNetwork/nex-protocols-go/v2/notifications/types"
)

const (
	// eagleNotification is the notification type the game's relay client watches for.
	eagleNotification notifications_constants.NotificationEvents = 200000

	// eagleTokenLifetime is how long a handoff token stays valid. Three hours is what earlier
	// research notes record: long enough that a whole session of back-to-back matches never
	// runs out, short enough that a leaked token is not permanent.
	eagleTokenLifetime = 3 * time.Hour

	// eagleServerEnv is the environment name the token carries. Retail is "lp1" (live
	// production 1) and the game is built against it, so it is not ours to choose.
	eagleServerEnv = "lp1"
)

// eagleToken is the signed envelope a player presents to the relay. The payload is a
// nested JSON object, not an encoded string, and the signature covers the payload exactly
// as it is serialised here.
type eagleToken struct {
	Payload   eagleTokenPayload `json:"payload"`
	Signature string            `json:"signature"`
	Version   int               `json:"version"`
}

// eagleTokenPayload is what the token says. Every field is a string on the wire, and two of
// them are formatted in a way that is not obvious: the gathering ID in decimal, the PID in
// hexadecimal padded to sixteen digits.
type eagleTokenPayload struct {
	ExpiresAt string `json:"expires_at"` // epoch seconds, decimal
	ServerEnv string `json:"server_env"`
	ServerID  string `json:"server_id"` // the gathering ID, decimal
	UserID    string `json:"user_id"`   // the PID, 16 hex digits
}

// buildEagleToken makes one player's handoff token. The token names the player, so it is
// built per recipient and cannot be broadcast to a lobby.
//
// The signature only has to convince our own relay — the client just carries the string —
// so the key is shared between this server and the relay and nothing else.
func buildEagleToken(key []byte, pid uint64, gid uint32, expires time.Time) (string, error) {
	payload := eagleTokenPayload{
		ExpiresAt: strconv.FormatInt(expires.Unix(), 10),
		ServerEnv: eagleServerEnv,
		ServerID:  strconv.FormatUint(uint64(gid), 10),
		UserID:    fmt.Sprintf("%016x", pid),
	}
	signed, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(signed)

	envelope, err := json.Marshal(eagleToken{
		Payload:   payload,
		Signature: base64.StdEncoding.EncodeToString(mac.Sum(nil)),
		Version:   1,
	})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(envelope), nil
}

// eagleURL is where a given gathering's relay session lives.
func eagleURL(base string, gid uint32) string {
	return fmt.Sprintf("%s/%d", base, gid)
}

// sendEagleHandoff tells the player who just joined a lobby where the relay is.
//
// The carrier is a revision 1 notification event: revision 0 has no map of parameters, and
// the map is exactly where the URL and the token travel.
func (s *Server) sendEagleHandoff(packet nex.PacketInterface) {
	if s.cfg.EagleURL == "" {
		return
	}
	connection, ok := packet.Sender().(*nex.PRUDPConnection)
	if !ok {
		return
	}
	pid := uint64(connection.PID())

	gid, ok := s.gatheringOf(connection.PID())
	if !ok {
		log.Printf("[smb35] no gathering for pid %s after a join: no Eagle handoff sent", redact.Tag(pid))
		return
	}

	token, err := buildEagleToken(s.cfg.EagleKey, pid, gid, time.Now().Add(eagleTokenLifetime))
	if err != nil {
		log.Printf("[smb35] eagle token for pid %s: %v", redact.Tag(pid), err)
		return
	}

	event := notifications_types.NewNotificationEvent()
	event.StructureVersion = 1 // the revision that carries MapParam
	event.PIDSource = s.host.Secure.ServerAccount.PID
	event.Type = eagleNotification
	event.Param1 = types.UInt64(gid)
	event.MapParam = types.Map[types.String, types.Variant]{
		"url":   {TypeID: types.NewUInt8(4), Type: types.NewString(eagleURL(s.cfg.EagleURL, gid))},
		"token": {TypeID: types.NewUInt8(4), Type: types.NewString(token)},
	}

	common_globals.SendNotificationEvent(s.host.Secure, event, []uint64{pid})
	log.Printf("[smb35] eagle handoff pid=%s gid=%d", redact.Tag(pid), gid)
}

// gatheringOf answers which gathering a player is currently in. The matchmaking tables are
// the only place that knows, and the common protocol has already written the join by the
// time a handoff is sent.
func (s *Server) gatheringOf(pid types.PID) (uint32, bool) {
	sessions, err := matchmake_extension_database.GetSimplePlayingSession(s.matchmaking, []types.PID{pid})
	if err != nil || len(sessions) == 0 {
		return 0, false
	}
	return uint32(sessions[0].GatheringID), true
}
