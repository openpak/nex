package nex

import (
	"os"
	"strconv"

	"github.com/PretendoNetwork/nex-go/v2/constants"
	"github.com/PretendoNetwork/nex-go/v2/types"
	commonticketgranting "github.com/PretendoNetwork/nex-protocols-common-go/v2/ticket-granting"
	ticketgranting "github.com/PretendoNetwork/nex-protocols-go/v2/ticket-granting"
	"github.com/PretendoNetwork/tetris-axis/globals"
)

func registerCommonAuthenticationServerProtocols() {
	ticketGrantingProtocol := ticketgranting.NewProtocol()
	globals.AuthenticationEndpoint.RegisterServiceProtocol(ticketGrantingProtocol)
	commonTicketGrantingProtocol := commonticketgranting.NewCommonProtocol(ticketGrantingProtocol)
	commonTicketGrantingProtocol.ConfigurePNValidation([]string{"00039E00"})

	port, _ := strconv.Atoi(os.Getenv("PN_TETR_SECURE_SERVER_PORT"))

	secureStationURL := types.NewStationURL("")
	secureStationURL.SetURLType(constants.StationURLPRUDPS)
	secureStationURL.SetAddress(os.Getenv("PN_TETR_SECURE_SERVER_HOST"))
	secureStationURL.SetPortNumber(uint16(port))
	secureStationURL.SetConnectionID(1)
	secureStationURL.SetPrincipalID(types.NewPID(2))
	secureStationURL.SetStreamID(1)
	secureStationURL.SetStreamType(constants.StreamTypeRVSecure)
	secureStationURL.SetType(uint8(constants.StationURLFlagPublic))

	commonTicketGrantingProtocol.SecureStationURL = secureStationURL
	commonTicketGrantingProtocol.BuildName = types.NewString("branch:trunk build:2_15_7221_0")
	commonTicketGrantingProtocol.SecureServerAccount = globals.SecureServerAccount
}
