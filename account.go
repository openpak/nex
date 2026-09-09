package nexhost

// Identity: turning a console into an OpenPak account.
//
// A Switch does not have a NEX password. It logs in with a BAAS id token, and nx-baas (the
// OpenPak Switch adapter) is the only thing that can prove one -- it holds the signing keys
// and this server deliberately does not. Its internal API returns the player's Switch
// projection. That is the same door servers/stardew-valley already uses for NPLN, so a
// player is the same person to a NEX title and an NPLN title.
//
// nex-go asks for accounts by PID or by username. The name a Switch logs in under is its NSA
// id, which is not its NEX PID and cannot be turned into one by arithmetic -- only the login
// saw both, so a name is resolved through the table the login filled in.

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
)

// ponytail: one client, no cache. Every login is one internal HTTP call on a LAN; add a
// short-TTL PID cache only if a title's login storm actually shows up in nx-baas latency.
var httpc = &http.Client{Timeout: 5 * time.Second}

// Player is nx-baas's view of a console's owner (its /internal/switch/identity reply). A
// title server reads Friends to answer friend-only matchmaking without its own friend graph.
type Player struct {
	PID        uint64   `json:"pid"`
	BaasUserID string   `json:"baas_user_id"` // the NSA id titles see
	Nickname   string   `json:"nickname"`
	Friends    []Friend `json:"friends"`
}

// Friend is one entry of the player's friend graph.
type Friend struct {
	PID        uint64 `json:"pid"`
	BaasUserID string `json:"baas_user_id"`
	Nickname   string `json:"nickname"`
}

// errTokenRejected means nx-baas refused the token itself, as opposed to not knowing the
// account. The two map to different NEX result codes: a rejected token makes the console
// fetch a fresh one, an unknown account must not.
var errTokenRejected = fmt.Errorf("token rejected")

// PlayerByToken proves a console's BAAS id token through nx-baas and returns its owner.
//
// Two shapes reach here. An emulator binds the account by adding a claim only nx-baas can
// sign, and that claim is the strong proof. A real console sends a plain BAAS id token with
// no such claim, so the whole token has to go to nx-baas and be proved there. We send both
// fields and let nx-baas decide which it can stand behind; it is the only thing holding the
// signing keys, and this server is deliberately not.
func (h *Host) PlayerByToken(idToken string) (*Player, error) {
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return nil, fmt.Errorf("no id token: %w", errTokenRejected)
	}
	query := map[string]any{"id_token": idToken}
	if claim := openpakClaim(idToken); claim != "" {
		query["openpak"] = claim
	}
	return h.identity(query)
}

// PlayerByPID looks up an already-proven PID (one that came out of a kerberos ticket).
func (h *Host) PlayerByPID(pid uint64) (*Player, error) {
	return h.identity(map[string]any{"pid": pid})
}

// openpakClaim pulls the nx-baas-signed binding claim out of a BAAS id token, or returns "" if
// there is none. An empty result is the normal case for a real console and is never an error
// here: it only means nx-baas will have to prove the token itself.
//
// A client that sends the bare claim instead of the whole JWT is passed through as-is.
func openpakClaim(idToken string) string {
	if strings.HasPrefix(idToken, "nx2.") {
		return idToken
	}
	segments := strings.Split(idToken, ".")
	if len(segments) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(segments[1], "="))
	if err != nil {
		return ""
	}
	var claims struct {
		OpenPak string `json:"openpak"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	return claims.OpenPak
}

func (h *Host) identity(query map[string]any) (*Player, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, h.Config.InternalURL+"/internal/switch/identity", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", h.Config.InternalKey)

	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("nx-baas: %s: %w", resp.Status, errTokenRejected)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nx-baas identity: %s", resp.Status)
	}
	var p Player
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	if p.PID == 0 || p.BaasUserID == "" {
		return nil, fmt.Errorf("nx-baas identity: incomplete reply")
	}
	return &p, nil
}

// accountByPID and accountByUsername are what nex-go calls. A user account carries
// RequiresTokenAuth, which tells nex-protocols-common-go never to derive a kerberos key
// from its password: on Switch the source key is handed to the client in the login
// response instead (see login.go), so the password below is never a shared secret.
func (h *Host) accountByPID(pid types.PID) (*nex.Account, *nex.Error) {
	if account := h.specialAccount(uint64(pid), ""); account != nil {
		return account, nil
	}
	player, err := h.PlayerByPID(uint64(pid))
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.RendezVous.InvalidPID, fmt.Sprintf("pid %d: %v", pid, err))
	}
	return userAccount(player), nil
}

func (h *Host) accountByUsername(username string) (*nex.Account, *nex.Error) {
	if account := h.specialAccount(0, username); account != nil {
		return account, nil
	}
	// The name a console logs in under is its NSA id, so the login's own table is the
	// authority. A decimal PID is accepted too: that is what a non-Switch client sends, and
	// what our own tooling uses.
	if pid, ok := PIDForLoginName(username); ok {
		return h.accountByPID(types.NewPID(pid))
	}
	pid, err := strconv.ParseUint(username, 10, 64)
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.RendezVous.InvalidUsername,
			fmt.Sprintf("username %q is neither a known login name nor a PID", username))
	}
	return h.accountByPID(types.NewPID(pid))
}

func (h *Host) specialAccount(pid uint64, username string) *nex.Account {
	for _, account := range []*nex.Account{h.authAccount, h.secureAccount} {
		if uint64(account.PID) == pid || account.Username == username {
			return account
		}
	}
	return nil
}

func userAccount(p *Player) *nex.Account {
	return nex.NewAccount(types.NewPID(p.PID), strconv.FormatUint(p.PID, 10), randomPassword(), true)
}

// randomPassword generates the 16 ASCII characters a NEX account password is. For special
// accounts it is the kerberos key material for tickets only this process reads back; for
// user accounts it is unused, because token logins carry an explicit source key.
func randomPassword() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err) // a server that cannot read the CSPRNG must not serve logins
	}
	return hex.EncodeToString(b)
}

// ---- login names ----
//
// A Switch logs in under its NSA id, and titles go on to ask for players by that same name
// rather than by NEX PID -- a DataStore search for "this console's own objects" arrives
// carrying the login name. Only the login saw both halves, so the translation is recorded
// there and read back here.
//
// ponytail: an in-process map, so a restart forgets it and a title asking about a player who
// has not logged in since gets a miss. Push it into nx-baas beside the account if that
// starts costing real lookups.
var loginNames sync.Map // login name -> PID

func (h *Host) rememberLoginName(name string, pid uint64) {
	if name != "" && pid != 0 {
		loginNames.Store(name, pid)
	}
}

// PIDForLoginName translates a name a console logged in under to its OpenPak PID.
func PIDForLoginName(name string) (uint64, bool) {
	pid, ok := loginNames.Load(name)
	if !ok {
		return 0, false
	}
	return pid.(uint64), true
}

// FriendPIDs answers "who are this player's friends". Two of Pretendo's common protocols ask
// for it by 32-bit PID and document it as not working on Switch, because upstream has no
// Switch friend graph to ask: DataStore's friends-only object permissions, and
// match-making's friend-only lobbies. OpenPak has the graph, so a title assigns this to both
// and the checks start working.
//
// Everything here fails closed. On a failed lookup it answers nothing rather than everything,
// so a friends-only object stays private and a friend-only lobby stays shut.
func (h *Host) FriendPIDs(pid uint32) []uint32 {
	player, err := h.PlayerByPID(uint64(pid))
	if err != nil {
		log.Printf("[%s] friend list for pid %d unavailable, treating as no friends: %v", h.Config.Title, pid, err)
		return nil
	}
	pids := make([]uint32, 0, len(player.Friends))
	for _, friend := range player.Friends {
		// The callback's PID is 32 bits and ours is 64. Truncating would not merely lose a
		// friend, it would name a different account -- 0x1_00000007 arrives as 7 -- and hand
		// that stranger a friend's access. Drop the entry instead. OpenPak PIDs are far below
		// this today, so the log line is the interesting part: it means PIDs have outgrown
		// the callbacks and every friends-only check is quietly losing people.
		if friend.PID == 0 || friend.PID > math.MaxUint32 {
			log.Printf("[%s] friend pid %d is not usable in the 32-bit friend callback, skipped", h.Config.Title, friend.PID)
			continue
		}
		pids = append(pids, uint32(friend.PID))
	}
	return pids
}
