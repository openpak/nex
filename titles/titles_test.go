package titles

import "testing"

func TestEveryTitleIsComplete(t *testing.T) {
	if len(All) != 17 {
		t.Fatalf("17 titles expected, %d registered", len(All))
	}
	for name, title := range All {
		if title.Defaults == nil || title.Register == nil {
			t.Errorf("%s: Defaults and Register are both required", name)
		}
	}
}
