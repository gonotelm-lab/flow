package scheduler

import (
	"encoding/binary"

	"github.com/gonotelm-lab/flow/server/internal/sharding"
	"github.com/google/uuid"
)

// ShardOf 把一个 schedule id 映射到静态分片号（0..1023）。
//
// 分片号在注册时算一次、写死进 schedules.shard，之后永不重算——所以扩缩容不需要
// 重写任何数据行，变的只是「分片 -> 实例」的映射（由实例注册表推导）。
//
// 取 id 的**后 8 字节**而不是前 2 字节：id 是 UUIDv7，前 48 位是毫秒时间戳，同一秒内
// 注册的 schedule 前几字节几乎完全相同，用它们取模会让大批记录挤进同一个分片。
// 后 8 字节来自随机部分，分布均匀。
func ShardOf(id uuid.UUID) int16 {
	return int16(binary.BigEndian.Uint64(id[8:16]) % uint64(sharding.ShardSize))
}
