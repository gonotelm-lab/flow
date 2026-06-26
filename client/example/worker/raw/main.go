// Raw worker：使用底层 []byte handler，演示 OkResult / ErrorResult 约定。
//
//	payload 以 "fail:" 开头 → ErrorResult
//	其他 payload → OkResult，内容为 "echo:" + payload
//
//	go run ./example/raw -addr localhost:7091 -namespace demo -task-type raw
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gonotelm-lab/flow/client/worker"
)

func main() {
	addr := flag.String("addr", "localhost:7091", "Flow worker gRPC 地址")
	namespace := flag.String("namespace", "demo", "任务 namespace")
	taskType := flag.String("task-type", "raw", "任务类型")
	name := flag.String("name", "raw-worker", "worker 名称")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	client, err := worker.New(*addr, worker.Config{
		Namespace: *namespace,
		TaskType:  *taskType,
		Name:      *name,
		Logger:    logger,
	})
	if err != nil {
		logger.Error("dial flow server failed", "addr", *addr, "err", err)
		os.Exit(1)
	}

	client.Handle(func(ctx context.Context, payload []byte) (worker.Result, error) {
		text := string(payload)
		logger.Info("handling raw task", "payload", text)

		if strings.HasPrefix(text, "fail:") {
			return worker.ErrorResult{Data: []byte(text)}, nil
		}
		return worker.OkResult{Data: []byte("echo:" + text)}, nil
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("raw worker starting", "addr", *addr, "namespace", *namespace, "task_type", *taskType)
	if err := client.Run(ctx); err != nil {
		logger.Error("worker stopped with error", "err", err)
		os.Exit(1)
	}
	logger.Info("worker stopped")
}
