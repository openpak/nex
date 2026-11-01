package histgrm

import (
	"reflect"
	"testing"
)

func TestDistribute(t *testing.T) {
	c := Distribute(9, []uint32{10, 10, 20, 100})
	if c.Total != 4 || c.Min != 10 || c.Max != 100 {
		t.Fatalf("%+v", c)
	}
	var sum uint32
	for _, b := range c.Buckets {
		sum += b
	}
	if sum != 4 || c.Buckets[0] != 2 || c.Buckets[3] != 1 || c.Buckets[Buckets-1] != 1 {
		t.Errorf("buckets %v", c.Buckets)
	}
	one := Distribute(1, []uint32{7})
	if one.Buckets[0] != 1 {
		t.Errorf("single score %v", one.Buckets)
	}
	if e := Distribute(2, nil); e.Total != 0 || len(e.Buckets) != Buckets {
		t.Errorf("empty %+v", e)
	}
	// the extreme of the uint32 range must not overflow
	big := Distribute(3, []uint32{0, 0xFFFFFFFF})
	if big.Buckets[0] != 1 || big.Buckets[Buckets-1] != 1 {
		t.Errorf("wide %v", big.Buckets)
	}
}

func TestBuildParseRoundTrip(t *testing.T) {
	in := []Category{Distribute(5, []uint32{1, 2, 3}), Distribute(2, []uint32{50})}
	b := Build(in)
	out, err := Parse(b)
	if err != nil || len(out) != 2 || out[0].ID != 2 {
		t.Fatalf("%v %+v", err, out)
	}
	if !reflect.DeepEqual(out[1], in[0]) {
		t.Errorf("round trip %+v vs %+v", out[1], in[0])
	}
	if !reflect.DeepEqual(Build(in), Build([]Category{in[1], in[0]})) {
		t.Error("output must not depend on input order")
	}
	if _, err := Parse(b[:len(b)-1]); err == nil {
		t.Error("truncated file must fail")
	}
}
