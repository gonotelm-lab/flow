package scheduler

import "github.com/gonotelm-lab/flow/api/signature"

// 回调请求头。
//
// 定义放在 api/signature 而不是这里：同一组头、同一套签名同时被服务端（签名）与
// 客户端 SDK（验签）使用，只能有一份定义，否则「发送方写错一个字母、接收方永远验不过」
// 这种问题只能靠线上发现。
const (
	// HeaderScheduleId 本次回调所属的 schedule id
	HeaderScheduleId = signature.HeaderScheduleID
	// HeaderFireId 本次 fire 的 id；同一次 fire 的所有重试与接管重投都不变，
	// 接收方必须用它做幂等去重——这是 at-least-once 契约的去重依据。
	HeaderFireId = signature.HeaderFireID
	// HeaderScheduledTime 本次触发计划时刻（unix 毫秒）
	HeaderScheduledTime = signature.HeaderScheduledTime
	// HeaderTimestamp 签名时间戳（unix 秒），参与签名以防重放
	HeaderTimestamp = signature.HeaderTimestamp
	// HeaderNamespace 本次回调所属的 namespace，同时也是签名 scope 的组成部分
	HeaderNamespace = signature.HeaderNamespace
	// HeaderSignature 签名值，形如 "FLOW-HMAC-SHA256 Scope=..., Signed=..., Signature=<hex>"
	HeaderSignature = signature.HeaderSignature
)

// 签名算法本身在 api/signature（Sign / Verify / Timestamp）。
// 调度器只负责把上面这些头按契约填齐，再调 signature.Sign——见 deliver。
