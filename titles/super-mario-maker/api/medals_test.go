package api

import "testing"

func TestMedals(t *testing.T) {
	for stars, want := range map[int]int{-5: 0, 0: 0, 1: 1, 49: 1, 50: 2, 150: 3, 4999: 9, 5000: 10, 1 << 20: 10} {
		if got := Medals(stars); got != want {
			t.Errorf("Medals(%d) = %d, want %d", stars, got, want)
		}
	}
}
