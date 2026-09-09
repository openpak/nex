package nexhost

// Identity: turning a console into an OpenPak account.
//
// A Switch does not have a NEX password. It logs in with a BAAS id token, and nx-baas (the
// OpenPak Switch adapter) is the only thing that can prove one: the id token carries an
// "openpak" claim signed with nx-baas's key, and POSTing that claim to nx-baas's internal API
// returns the player's Switch projection. That is the same door servers/stardew-valley
// already uses for NPLN, so a player is the same person to a NEX title and an NPLN title.
//
// nex-go asks for accounts by PID or by username. On Switch the NEX username is the PID as
// a decimal string, so both lookups end at the same place: nx-baas.

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
func (h *Host) PlayerByToken(idToken string) (*Player, error) {
	claim, err := openpakClaim(idToken)
	if err != nil {
		return nil, err
	}
	return h.identity(map[string]any{"openpak": claim})
}

// PlayerByPID looks up an already-proven PID (one that came out of a kerberos ticket).
func (h *Host) PlayerByPID(pid uint64) (*Player, error) {
	return h.identity(map[string]any{"pid": pid})
}

// openpakClaim pulls the nx-baas-signed "openpak" claim out of a BAAS id token. A client that
// sends the bare claim instead of the whole JWT is accepted as-is; nx-baas proves the
// signature either way, so this only decides where to look, never whether to trust.
func openpakClaim(idToken string) (string, error) {
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return "", fmt.Errorf("no id token: %w", errTokenRejected)
	}
	segments := strings.Split(idToken, ".")
	if len(segments) != 3 || strings.HasPrefix(idToken, "nx2.") {
		return idToken, nil // already the openpak token
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(segments[1], "="))
	if err != nil {
		return "", fmt.Errorf("id token payload is not base64url: %w", errTokenRejected)
	}
	var claims struct {
		OpenPak string `json:"openpak"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.OpenPak == "" {
		return "", fmt.Errorf("id token has no openpak claim: %w", errTokenRejected)
	}
	return claims.OpenPak, nil
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
	// A Switch sends its PID as the username.
	pid, err := strconv.ParseUint(username, 10, 64)
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.RendezVous.InvalidUsername, fmt.Sprintf("username %q is not a PID", username))
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
