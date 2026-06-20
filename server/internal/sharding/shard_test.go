package sharding

import (
	"testing"
)

func TestSequentialCalculator_GetShard(t *testing.T) {
	calculator := &SequentialCalculator{}

	for i := range 5 {
		shard := calculator.GetShard(int64(i+1), []int64{1, 2, 3, 4, 5})
		t.Logf("shard: %+v", shard)
	}

	t.Log("--------------------------------")

	for i := range 16 {
		shard := calculator.GetShard(int64(i+1), []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		t.Logf("shard: %+v", shard)
	}

	t.Log("--------------------------------")

	for i := range 4 {
		shard := calculator.GetShard(int64(i+1), []int64{1, 2, 3, 4})
		t.Logf("shard: %+v", shard)
	}
	
	t.Log("--------------------------------")
	shard := calculator.GetShard(1, []int64{1})
	t.Logf("shard: %+v", shard)
}
