package mmlegacy

import (
	"testing"

	common_globals "github.com/PretendoNetwork/nex-protocols-common-go/v2/globals"
	match_making "github.com/PretendoNetwork/nex-protocols-go/v2/match-making"
	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
)

// TestRegisterSetsEveryMethod: an unset handler is not a compile error, it is a method that
// answers Core::NotImplemented -- silent on this side, a dead end on the console. This is
// the check that a renamed or dropped assignment is caught here rather than on hardware.
func TestRegisterSetsEveryMethod(t *testing.T) {
	matchMaking := match_making.NewProtocol()
	matchmakeExtension := matchmake_extension.NewProtocol()

	Register(matchMaking, matchmakeExtension, &common_globals.MatchmakingManager{})

	for name, set := range map[string]bool{
		"MatchMaking.FindByOwner":                                        matchMaking.FindByOwner != nil,
		"MatchmakeExtension.FindMatchmakeSessionByParticipant":           matchmakeExtension.FindMatchmakeSessionByParticipant != nil,
		"MatchmakeExtension.FindMatchmakeSessionBySingleGatheringID":     matchmakeExtension.FindMatchmakeSessionBySingleGatheringID != nil,
		"MatchmakeExtension.BrowseMatchmakeSessionNoHolderNoResultRange": matchmakeExtension.BrowseMatchmakeSessionNoHolderNoResultRange != nil,
		"MatchmakeExtension.UpdateMatchmakeSessionAttribute":             matchmakeExtension.UpdateMatchmakeSessionAttribute != nil,
	} {
		if !set {
			t.Errorf("%s is still unset: the title will answer Core::NotImplemented", name)
		}
	}
}
