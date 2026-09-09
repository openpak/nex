package nexhost

// The Switch ticket-granting methods.
//
// The protocol has one set of method IDs with two sets of names (Kinnay's NintendoClients
// wiki, Authentication Protocol):
//
//	ID  Wii U / 3DS       Switch (NEX 4)
//	1   Login             ValidateAndRequestTicket
//	2   LoginEx           ValidateAndRequestTicketWithCustomData
//	6   LoginWithContext  ValidateAndRequestTicketWithParam
//
// nex-protocols-common-go implements 1, 2 and 3 under the Wii U names, and nex-protocols-go
// routes to the Switch names whenever the library version is >= 4.0.0 — so on a Switch title
// none of the common handlers are reachable and the server answers "not implemented" to
// every login. That is the gap this file fills.
//
// A Switch has no NEX password. It sends a BAAS id token in AuthenticationInfo, and the
// server answers with a sourceKey field: "Kerberos key. If present, key derivation is
// skipped and this key is used to decrypt the ticket instead." So the source key is ours to
// choose, and we choose fresh random bytes per login. Nothing derived from an account
// password ever crosses the wire.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"

	nex "github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	ticket_granting "github.com/PretendoNetwork/nex-protocols-go/v2/ticket-granting"
	ticket_granting_types "github.com/PretendoNetwork/nex-protocols-go/v2/ticket-granting/types"
)

// sourceKeyLength is how many random bytes the source key is — the key a Switch is handed as
// pSourceKey and decrypts its ticket with.
//
// The invariant that actually matters is that the key we encrypt the ticket with is the key
// we advertise. RC4 takes any length and the console is told the key rather than deriving it,
// so the length is free as long as the two agree. Both 16 and 32 are attested by servers
// known to work against retail consoles, which is why this is not a protocol constant: a
// mismatch between the two halves is what rejects a ticket, not the number itself.
//
// 32 because it is the larger attested value and nothing is known to reject it.
const sourceKeyLength = 32

// validateAndRequestTicketWithParam is method 6 on NEX 4.4.0 and later.
func (h *Host) validateAndRequestTicketWithParam(err error, packet nex.PacketInterface, callID uint32, param ticket_granting_types.ValidateAndRequestTicketParam) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	}
	endpoint := packet.Sender().Endpoint().(*nex.PRUDPEndPoint)

	pid, sourceKey, ticket, nexErr := h.authorize(endpoint, string(param.Username), param.ExtraData)
	if nexErr != nil {
		return nil, nexErr
	}

	result := ticket_granting_types.NewValidateAndRequestTicketResult()
	// The structure header matters: a client handed the flat method-2 layout, or a header
	// with the wrong version, rejects the login with Core::InvalidArgument (the console
	// shows 2306-0116) rather than failing anywhere informative.
	result.StructureVersion = 1
	result.SourcePID = types.NewPID(pid)
	result.BufResponse = types.NewBuffer(ticket)
	result.ServiceNodeURL = h.SecureStationURL
	result.CurrentUTCTime = types.NewDateTime(0).Now()
	result.ReturnMsg = types.NewString("OpenPak " + h.Config.Title)
	result.SourceKey = types.NewString(hex.EncodeToString(sourceKey))

	stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	result.WriteTo(stream)

	response := nex.NewRMCSuccess(endpoint, stream.Bytes())
	response.ProtocolID = ticket_granting.ProtocolID
	response.MethodID = ticket_granting.MethodLoginWithContext
	response.CallID = callID
	return response, nil
}

// validateAndRequestTicketWithCustomData is method 2, the shape used before NEX 4.4.0. Same
// authorization, the older flat response with pSourceKey appended.
func (h *Host) validateAndRequestTicketWithCustomData(err error, packet nex.PacketInterface, callID uint32, strUserName types.String, oExtraData types.DataHolder) (*nex.RMCMessage, *nex.Error) {
	if err != nil {
		return nil, nex.NewError(nex.ResultCodes.Core.InvalidArgument, err.Error())
	}
	endpoint := packet.Sender().Endpoint().(*nex.PRUDPEndPoint)

	pid, sourceKey, ticket, nexErr := h.authorize(endpoint, string(strUserName), oExtraData)
	if nexErr != nil {
		return nil, nexErr
	}

	connectionData := types.NewRVConnectionData()
	connectionData.StationURL = h.SecureStationURL
	connectionData.SpecialProtocols = types.List[types.UInt8]{}
	connectionData.StationURLSpecialProtocols = types.NewStationURL("")
	connectionData.Time = types.NewDateTime(0).Now()
	connectionData.StructureVersion = 1

	stream := nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings())
	types.NewQResultSuccess(nex.ResultCodes.Core.Unknown).WriteTo(stream)
	types.NewPID(pid).WriteTo(stream)
	types.NewBuffer(ticket).WriteTo(stream)
	connectionData.WriteTo(stream)
	types.NewString("OpenPak " + h.Config.Title).WriteTo(stream)
	types.NewString(hex.EncodeToString(sourceKey)).WriteTo(stream)

	response := nex.NewRMCSuccess(endpoint, stream.Bytes())
	response.ProtocolID = ticket_granting.ProtocolID
	response.MethodID = ticket_granting.MethodLoginEx
	response.CallID = callID
	return response, nil
}

// authorize proves the console's token, checks it against the username it claims, and mints
// the ticket for the secure server. Fail-closed: no token, no ticket.
func (h *Host) authorize(endpoint *nex.PRUDPEndPoint, username string, extraData types.DataHolder) (uint64, []byte, []byte, *nex.Error) {
	token, ok := authenticationToken(extraData)
	if !ok {
		// NullData, or a login data type we have never seen. Either way there is nothing
		// to prove, and a Switch title has no password path to fall back to.
		return 0, nil, nil, nex.NewError(nex.ResultCodes.Authentication.ValidationFailed, "login carried no authentication token")
	}

	player, err := h.PlayerByToken(token)
	if err != nil {
		log.Printf("[%s] login refused: %v", h.Config.Title, err)
		if errors.Is(err, errTokenRejected) {
			// The console reacts to TokenExpired by fetching a fresh token, which is the
			// right move for a token we would not prove.
			return 0, nil, nil, nex.NewError(nex.ResultCodes.Authentication.TokenExpired, "id token not proven")
		}
		return 0, nil, nil, nex.NewError(nex.ResultCodes.Authentication.ValidationFailed, "no OpenPak account for this console")
	}

	// The name a Switch logs in under is its NSA id, not its NEX PID, so the two never
	// match and comparing them would refuse every real console. The token is the only
	// authority on identity. Remember the name instead: titles go on to ask for players by
	// the name they logged in with rather than by PID, and nothing else can translate it.
	h.rememberLoginName(username, player.PID)

	sourceKey := make([]byte, sourceKeyLength)
	if _, keyErr := rand.Read(sourceKey); keyErr != nil {
		return 0, nil, nil, nex.NewError(nex.ResultCodes.Authentication.Unknown, "failed to generate source key")
	}

	ticket, ticketErr := issueTicket(endpoint, types.NewPID(player.PID), h.secureAccount, sourceKey)
	if ticketErr != nil {
		return 0, nil, nil, nex.NewError(nex.ResultCodes.Authentication.Unknown, ticketErr.Error())
	}

	log.Printf("[%s] login pid=%d nickname=%q friends=%d", h.Config.Title, player.PID, player.Nickname, len(player.Friends))
	return player.PID, sourceKey, ticket, nil
}

// authenticationToken reads the id token out of the login data. Switch logins carry it in
// AuthenticationInfo; NullData means no token was sent at all.
func authenticationToken(extraData types.DataHolder) (string, bool) {
	if extraData.Object == nil {
		return "", false
	}
	info, ok := extraData.Object.Copy().(ticket_granting_types.AuthenticationInfo)
	if !ok || info.Token == "" {
		return "", false
	}
	return string(info.Token), true
}

// issueTicket builds the kerberos ticket the console forwards to the secure server. The
// equivalent in nex-protocols-common-go is unexported and reachable only from the Wii U
// handlers, so this is the same construction against nex-go's primitives: a session key
// encrypted to the caller under sourceKey, wrapping an internal ticket encrypted to the
// secure server under its own key.
func issueTicket(endpoint *nex.PRUDPEndPoint, sourcePID types.PID, target *nex.Account, sourceKey []byte) ([]byte, error) {
	server := endpoint.Server

	sessionKey := make([]byte, server.SessionKeyLength)
	if _, err := rand.Read(sessionKey); err != nil {
		return nil, err
	}

	internal := nex.NewKerberosTicketInternalData(server)
	internal.Issued = types.NewDateTime(0).Now()
	internal.SourcePID = sourcePID
	internal.SessionKey = sessionKey

	targetKey := nex.DeriveKerberosKey(target.PID, []byte(target.Password))
	encryptedInternal, err := internal.Encrypt(targetKey, nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings()))
	if err != nil {
		return nil, err
	}

	ticket := nex.NewKerberosTicket()
	ticket.SessionKey = sessionKey
	ticket.TargetPID = target.PID
	ticket.InternalData = types.NewBuffer(encryptedInternal)
	return ticket.Encrypt(sourceKey, nex.NewByteStreamOut(endpoint.LibraryVersions(), endpoint.ByteStreamSettings()))
}
