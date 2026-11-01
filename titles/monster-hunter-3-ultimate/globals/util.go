package globals

import (
	"unicode/utf8"

	"github.com/PretendoNetwork/nex-go/v2"
	"github.com/PretendoNetwork/nex-go/v2/types"
	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	messaging_constants "github.com/PretendoNetwork/nex-protocols-go/v2/messaging/constants"
	messaging_types "github.com/PretendoNetwork/nex-protocols-go/v2/messaging/types"
)

// ValidateUserMessage is a basic message validator that only checks the subject.
// By default, all fields are empty except for the Subject, Message Recipient, and Message Type.
// For now, we are not checking those as we do not know what short messages (currently inaccessible) look like
func ValidateUserMessage(userMessage types.DataHolder) (types.UInt64, messaging_constants.RecipientType, *nex.Error) {
	message := userMessage.Object.(messaging_types.UserMessage)

	if utf8.RuneCountInString(string(message.StrSubject)) > 256 {
		return 0, 0, nex.NewError(nex.ResultCodes.Core.InvalidArgument, "Message subject is too long")
	}

	recipientID, recipientType := common_globals.GetUserMessageRecipientData(SecureEndpoint.Server.LibraryVersions.Messaging, message)
	return recipientID, recipientType, nil
}


