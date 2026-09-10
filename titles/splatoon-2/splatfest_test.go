package splatoon2

import (
	"testing"

	matchmake_extension "github.com/PretendoNetwork/nex-protocols-go/v2/matchmake-extension"
	ranking "github.com/PretendoNetwork/nex-protocols-go/v2/ranking"
	utility "github.com/PretendoNetwork/nex-protocols-go/v2/utility"
)

// Every method named in PatchedMethods must have a handler, and every handler must be
// named in PatchedMethods. nex-go routes only PatchedMethods to the patch, so a method
// listed without a handler reaches the fallback and a handler not listed is dead code:
// either way the two lists have drifted, which is exactly the bug this catches.
func TestPatchedMethodsMatchTheirHandlers(t *testing.T) {
	utilityProtocol := utility.NewProtocol()
	patchUtility(utilityProtocol)
	assertConsistent(t, "utility", utilityProtocol.Patches, utilityProtocol.PatchedMethods)

	rankingProtocol := ranking.NewProtocol()
	patchRanking(rankingProtocol)
	assertConsistent(t, "ranking", rankingProtocol.Patches, rankingProtocol.PatchedMethods)

	extension := matchmake_extension.NewProtocol()
	patchMatchmakeExtension(extension)
	assertConsistent(t, "matchmake-extension", extension.Patches, extension.PatchedMethods)
}

func assertConsistent(t *testing.T, name string, patches any, methods []uint32) {
	t.Helper()
	patch, ok := patches.(*methodPatch)
	if !ok {
		t.Fatalf("%s: Patches is not a *methodPatch", name)
	}
	if len(methods) != len(patch.handlers) {
		t.Fatalf("%s: %d patched methods but %d handlers", name, len(methods), len(patch.handlers))
	}
	for _, method := range methods {
		if _, has := patch.handlers[method]; !has {
			t.Errorf("%s: method %d is patched but has no handler", name, method)
		}
	}
}
