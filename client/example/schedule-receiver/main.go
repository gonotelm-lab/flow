// 定时回调接收方示例：一条调用完成验签 + 幂等去重，是可以直接抄进生产 handler 的骨架。
//
// 需要先启动 flowserver（默认 gRPC 在 7091），再启动本示例：
//
//	cd client && go run ./example/schedule-receiver -addr :8080 -api-key "$FLOW_API_KEY"
//
// 然后注册一条指向它的 schedule：
//
//	cd client && go run ./example/schedule -callback http://127.0.0.1:8080/hook \
//	  -namespace demo -cron "*/1 * * * *"
//
// 接收方要做的三件事，本示例挨个演示：
//
//  1. **验签**：schedule.VerifyRequest 一行搞定（读 body、复算签名、判时间戳新鲜度，
//     并把 body 还原回 r.Body）。api_key 用该 namespace 的 api_key。
//  2. **去重**：投递是 at-least-once，同一次 fire 可能重投。用 X-Flow-Fire-Id 做唯一键，
//     重复的直接回 2xx（回 4xx/5xx 只会让对方继续重试）。
//  3. **快回**：先落业务，再回 200；服务端超时是 10s，超时即算失败并进入重试。
package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"sync"

	scheduleclient "github.com/gonotelm-lab/flow/client/schedule"
)

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	path := flag.String("path", "/hook", "回调路径")
	apiKey := flag.String("api-key", "", "namespace 的 api_key（必填，用于验签）")
	namespace := flag.String("namespace", "", "可选：限定只接受该 namespace 的回调")
	flag.Parse()

	if *apiKey == "" {
		log.Fatal("-api-key 必填（namespace 的 api_key）")
	}

	recv := &receiver{seen: map[string]struct{}{}}

	// 接收方只用验签，地址留空即可：gRPC 懒连接，不调接口就不会真的建连。
	// namespace 可选：配了就顺带校验 X-Flow-Namespace。
	verifier, err := scheduleclient.New("", scheduleclient.Config{
		Namespace: *namespace,
		APIKey:    *apiKey,
	})
	if err != nil {
		log.Fatalf("构造客户端失败: %v", err)
	}
	defer func() { _ = verifier.Close() }()

	mux := http.NewServeMux()
	mux.HandleFunc(*path, recv.handle(verifier))

	log.Printf("接收方已启动: http://127.0.0.1%s%s", *addr, *path)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

type receiver struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func (r *receiver) handle(verifier *scheduleclient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// 1. 验签。失败一律不回 200，否则服务端会当成投递成功。
		//    body 在这一步之后仍可读（VerifyRequest 读完会还原）。
		if err := verifier.VerifyRequest(req); err != nil {
			log.Printf("验签失败: %v", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			log.Printf("读 body 失败: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// 2. 幂等去重：同一次 fire 的所有重试与接管重投共用同一个 Fire-Id。
		//    生产环境应当把它写进唯一索引（而不是这个进程内 map）。
		fireID := req.Header.Get(scheduleclient.HeaderFireID)
		if r.markSeen(fireID) {
			log.Printf("重复投递，已忽略: schedule=%s fire=%s",
				req.Header.Get(scheduleclient.HeaderScheduleID), fireID)
			w.WriteHeader(http.StatusOK)
			return
		}

		// 3. 真正的业务处理放在这里。
		log.Printf("收到回调: schedule=%s fire=%s scheduled_time=%s namespace=%s body=%s",
			req.Header.Get(scheduleclient.HeaderScheduleID),
			fireID,
			req.Header.Get(scheduleclient.HeaderScheduledTime),
			req.Header.Get(scheduleclient.HeaderNamespace),
			body,
		)

		w.WriteHeader(http.StatusOK)
	}
}

// markSeen 返回 true 表示这个 fire 已经处理过。
func (r *receiver) markSeen(fireID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.seen[fireID]; ok {
		return true
	}
	r.seen[fireID] = struct{}{}
	return false
}
