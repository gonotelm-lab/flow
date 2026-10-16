package sharding

import "slices"

const (
	// 分片数量
	// 分片编号从 0 到 1023
	ShardSize = 1024 // DO NOT CHANGE THIS VALUE
)

type Shard struct {
	Id int64

	// 分片区间为 [Start, End]
	Start int
	End   int
}

func (s *Shard) Valid() bool {
	return s.Start >= 0 && s.End < ShardSize
}

type Calculator interface {
	GetShard(
		current int64,
		others []int64,
	) *Shard
}

// 按排序后的顺序计算 shard
type SequentialCalculator struct{}

func (c *SequentialCalculator) GetShard(
	current int64,
	others []int64,
) *Shard {
	slices.Sort(others)
	index := slices.Index(others, current)
	if index == -1 {
		return &Shard{
			Id:    current,
			Start: 0,
			End:   ShardSize - 1,
		}
	}

	shardSize := ShardSize / len(others)
	start := index * shardSize
	end := start + shardSize - 1
	// 最后一个分片把剩下的全占了
	if index == len(others)-1 {
		end = ShardSize - 1
	}
	return &Shard{
		Id:    current,
		Start: start,
		End:   end,
	}
}
