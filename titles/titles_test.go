package titles

import "testing"

func TestEveryTitleIsComplete(t *testing.T) {
	if len(All) != 59 {
		t.Fatalf("59 titles expected, %d registered", len(All))
	}
	for name, title := range All {
		if title.Defaults == nil || (title.Register == nil) == (title.Run == nil) {
			t.Errorf("%s: Defaults and exactly one of Register or Run are required", name)
		}
	}
}
