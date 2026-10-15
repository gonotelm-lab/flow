// 定时回调客户端示例：注册一条 schedule，并打印服务端返回的运行态。
//
// 需要先启动 flowserver（默认 gRPC 在 7091）：
//
//	cd server && go run ./cmd/flowserver
//
// 然后：
//
//	# 每 5 分钟回调一次
//	go run ./example/schedule -namespace demo -callback http://127.0.0.1:8080/hook
//
//	# 10 秒后触发一次
//	go run ./example/schedule -namespace demo -callback http://127.0.0.1:8080/hook -once
//
//	# 带幂等键：重复执行只会注册出一条
//	go run ./example/schedule -namespace demo -callback http://127.0.0.1:8080/hook \
//	  -idempotency-key nightly-report
package main

import (
	"context"
	"flag"
	"log"
	"time"

	scheduleclient "github.com/gonotelm-lab/flow/client/schedule"
)

func main() {
	addr := flag.String("addr", "localhost:7091", "flow server gRPC 地址")
	namespace := flag.String("namespace", "demo", "namespace")
	callback := flag.String("callback", "", "回调 URL（必填，http/https）")
	cron := flag.String("cron", "*/5 * * * *", "标准 5 字段 cron 表达式")
	timezone := flag.String("timezone", "UTC", "IANA 时区名，如 Asia/Shanghai")
	once := flag.Bool("once", false, "注册一次性任务而不是 cron")
	after := flag.Duration("after", 10*time.Second, "一次性任务的延迟")
	payload := flag.String("payload", `{"hello":"world"}`, "回调 body 的原始内容")
	contentType := flag.String("content-type", "application/json", "回调 Content-Type")
	idempotencyKey := flag.String("idempotency-key", "", "可选幂等键（namespace 内唯一）")
	flag.Parse()

	if *callback == "" {
		log.Fatal("-callback 必填")
	}

	client, err := scheduleclient.New(*addr)
	if err != nil {
		log.Fatalf("连接 %s 失败: %v", *addr, err)
	}
	defer func() { _ = client.Close() }()

	opts := []scheduleclient.Option{
		scheduleclient.WithNamespace(*namespace),
		scheduleclient.WithContentType(*contentType),
	}
	if *idempotencyKey != "" {
		opts = append(opts, scheduleclient.WithIdempotencyKey(*idempotencyKey))
	}

	// 用便利构造函数，不必手拼 proto 的 oneof
	var s = scheduleclient.Cron(*cron, *timezone, *callback, []byte(*payload), opts...)
	if *once {
		s = scheduleclient.After(*after, *callback, []byte(*payload), opts...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created, err := client.Register(ctx, s)
	if err != nil {
		log.Fatalf("注册失败: %v", err)
	}

	log.Printf("已注册 schedule id=%s", created.GetId())
	if created.GetCron() != nil {
		log.Printf("  触发方式: cron %q (timezone=%s)",
			created.GetCron().GetExpr(), created.GetCron().GetTimezone())
	} else if created.GetOneShot() != nil {
		log.Printf("  触发方式: 一次性 %s",
			time.UnixMilli(created.GetOneShot().GetFireAt()).Format(time.RFC3339))
	}
	log.Printf("  下次触发: %s", time.UnixMilli(created.GetNextFireAt()).Format(time.RFC3339))
	log.Printf("  回调地址: %s", created.GetCallbackUrl())
	log.Printf("  当前状态: %s", created.GetState())

	log.Print("提示：回调是 at-least-once 的，接收方请用 X-Flow-Fire-Id 做幂等去重；")
	log.Print("      并用 X-Flow-Signature（HMAC-SHA256，密钥为 namespace 的 api_key）校验来源。")
}
