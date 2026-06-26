// Echo worker：连接 Flow server，消费 namespace/task_type 下的任务并回写大写消息。
//
// 运行前请确保 Flow server 已启动，且目标 namespace 已通过 Admin API 创建。
//
//	go run ./example/echo \
//	  -addr localhost:7091 \
//	  -namespace demo \
//	  -task-type echo
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

type echoInput struct {
	Message string `json:"message"`
}

type echoOutput struct {
	Message string `json:"message"`
}

func main() {
	addr := flag.String("addr", "localhost:7091", "Flow worker gRPC 地址")
	namespace := flag.String("namespace", "demo", "任务 namespace")
	taskType := flag.String("task-type", "echo", "任务类型")
	name := flag.String("name", "echo-worker", "worker 名称")
	concurrency := flag.Int("concurrency", 2, "最大并发任务数")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	client, err := worker.New(*addr, worker.Config{
		Namespace:      *namespace,
		TaskType:       *taskType,
		Name:           *name,
		MaxConcurrency: *concurrency,
		Logger:         logger,
	})
	if err != nil {
		logger.Error("dial flow server failed", "addr", *addr, "err", err)
		os.Exit(1)
	}

	worker.RegisterTyped[echoInput, echoOutput](client, func(ctx context.Context, in echoInput) (echoOutput, error) {
		logger.Info("handling task", "message", in.Message)
		return echoOutput{Message: strings.ToUpper(in.Message)}, nil
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("worker starting",
		"addr", *addr,
		"namespace", *namespace,
		"task_type", *taskType,
		"concurrency", *concurrency,
	)

	if err := client.Run(ctx); err != nil {
		logger.Error("worker stopped with error", "err", err)
		os.Exit(1)
	}
	logger.Info("worker stopped")
}
