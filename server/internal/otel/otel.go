package otel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	noop "go.opentelemetry.io/otel/trace/noop"

	"github.com/gonotelm-lab/flow/server/internal/config"
)

var (
	mu          sync.Mutex
	initialized bool
	provider    *sdktrace.TracerProvider
)

// Init 初始化 server 自身的 OpenTelemetry SDK 并设置全局 provider。
// 显式配置优先（[otel] TOML 段），未配置项回落到 OTEL_* 环境变量标准（SDK 原生支持）。
func Init(ctx context.Context, cfg *config.OtelConfig) error {
	o := &options{}
	if cfg != nil {
		o.serviceName = cfg.ServiceName
		o.endpoint = cfg.Endpoint
		o.protocol = Protocol(cfg.Protocol)
		if cfg.SamplerRatio > 0 {
			o.samplerRatio = cfg.SamplerRatio
		}
	}

	exporter, err := newExporter(ctx, o)
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	if initialized {
		slog.InfoContext(ctx, "[otel] already initialized, skip")
		return nil
	}

	var attrs []attribute.KeyValue
	if o.serviceName != "" {
		attrs = append(attrs, semconv.ServiceName(o.serviceName))
	}
	res := resource.NewWithAttributes(semconv.SchemaURL, attrs...)

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	}
	// 仅当显式指定采样率时覆盖环境变量；否则 OTEL_TRACES_SAMPLER 由 SDK 原生处理
	if o.samplerRatio > 0 {
		tpOpts = append(tpOpts, sdktrace.WithSampler(
			sdktrace.ParentBased(sdktrace.TraceIDRatioBased(o.samplerRatio)),
		))
	}

	provider = sdktrace.NewTracerProvider(tpOpts...)
	initialized = true

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.InfoContext(ctx, "[otel] tracer provider initialized",
		slog.String("serviceName", o.serviceName),
		slog.String("protocol", string(o.protocol)),
		slog.String("endpoint", o.endpoint),
	)
	return nil
}

// Shutdown 刷新并关闭 TracerProvider。幂等，可重入（之后可再次 Init）。
func Shutdown(ctx context.Context) {
	mu.Lock()
	defer mu.Unlock()
	if provider == nil {
		return
	}
	if err := provider.Shutdown(ctx); err != nil {
		slog.ErrorContext(ctx, "[otel] shutdown failed", slog.Any("err", err))
		return
	}
	slog.InfoContext(ctx, "[otel] shutdown complete")
	provider = nil
	initialized = false
	otel.SetTracerProvider(noop.NewTracerProvider())
}

// TraceparentFromContext 用 W3C TraceContext（固定，不依赖全局 propagator 配置）从 ctx 提取 traceparent；无 span 时返回空串。
func TraceparentFromContext(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	traceContextPropagator.Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

type options struct {
	serviceName  string
	endpoint     string
	protocol     Protocol
	samplerRatio float64
}

type Protocol string

const (
	ProtocolGRPC Protocol = "grpc"
	ProtocolHTTP Protocol = "http"
)

// traceContextPropagator 固定使用 W3C TraceContext。
var traceContextPropagator = propagation.TraceContext{}

// newExporter 按 protocol 创建 OTLP exporter。protocol 优先级：配置 > OTEL_EXPORTER_OTLP_PROTOCOL > grpc。
// endpoint 为空时由 exporter 原生读取 OTEL_EXPORTER_OTLP_ENDPOINT。
func newExporter(ctx context.Context, o *options) (sdktrace.SpanExporter, error) {
	protocol := o.protocol
	if protocol == "" {
		protocol = ProtocolGRPC
	}

	switch protocol {
	case ProtocolGRPC:
		// gRPC 导出器默认要求 TLS；内部部署默认走 insecure
		opts := []otlptracegrpc.Option{otlptracegrpc.WithInsecure()}
		if o.endpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(o.endpoint))
		}
		return otlptracegrpc.New(ctx, opts...)
	case ProtocolHTTP:
		var opts []otlptracehttp.Option
		if o.endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(o.endpoint))
		}
		return otlptracehttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("unsupported otlp protocol: %q", protocol)
	}
}
