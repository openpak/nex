package nex

import (
	"os"
	"strconv"

	"github.com/openpak/nex/titles/monster-hunter-xx/globals"
	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/constants"
	"github.com/PretendoNetwork/nex-go/v2/types"
	commonticketgranting "github.com/PretendoNetwork/nex-protocols-common-go/v2/ticket-granting"
	ticketgranting "github.com/PretendoNetwork/nex-protocols-go/v2/ticket-granting"
)

func registerCommonAuthenticationServerProtocols() {
	ticketGrantingProtocol := ticketgranting.NewProtocol()
	// upstream's dev branch called SetUseCrossplay(true) on a nex-protocols-go fork; the released
	// library has no such knob, so the line is dropped.
	globals.AuthenticationEndpoint.RegisterServiceProtocol(ticketGrantingProtocol)
	commonTicketGrantingProtocol := commonticketgranting.NewCommonProtocol(ticketGrantingProtocol)
	// HACK: replace with ConfigurePNValidation once MHXX branch is rebased up to latest
	commonTicketGrantingProtocol.ValidateLoginData = func(pid types.PID, loginData types.DataHolder) *nex.Error {
		return nil
	}

	port, _ := strconv.Atoi(os.Getenv("PN_MHXX_SECURE_SERVER_PORT"))

	secureStationURL := types.NewStationURL("")
	secureStationURL.SetURLType(constants.StationURLPRUDPS)
	secureStationURL.SetAddress(os.Getenv("PN_MHXX_SECURE_SERVER_HOST"))
	secureStationURL.SetPortNumber(uint16(port))
	secureStationURL.SetConnectionID(1)
	secureStationURL.SetPrincipalID(types.NewPID(2))
	secureStationURL.SetStreamID(1)
	secureStationURL.SetStreamType(constants.StreamTypeRVSecure)
	secureStationURL.SetType(uint8(constants.StationURLFlagPublic))

	commonTicketGrantingProtocol.SecureStationURL = secureStationURL
	commonTicketGrantingProtocol.BuildName = types.NewString("branch:origin/project/ctr-agq build:4_6_19_0_0")
	commonTicketGrantingProtocol.SecureServerAccount = globals.SecureServerAccount
}
