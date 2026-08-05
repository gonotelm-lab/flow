package otel

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
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
	initOnce sync.Once
	initErr  error
	mu       sync.Mutex
	provider *sdktrace.TracerProvider
)

// Init 初始化 server 自身的 OpenTelemetry SDK 并设置全局 provider，只执行一次。
// 显式配置优先（[otel] TOML 段），未配置项回落到 OTEL_* 环境变量标准。
func Init(ctx context.Context, cfg *config.OtelConfig) error {
	initOnce.Do(func() { initErr = initProvider(ctx, cfg) })
	return initErr
}

func initProvider(ctx context.Context, cfg *config.OtelConfig) error {
	o := &options{}
	if cfg != nil {
		o.serviceName = cfg.ServiceName
		o.endpoint = cfg.Endpoint
		o.protocol = Protocol(cfg.Protocol)
		o.samplerRatio = cfg.SamplerRatio
	}

	exporter, err := newExporter(ctx, o)
	if err != nil {
		return err
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
	// sampler 优先级：显式 samplerRatio（OTel traceidratio 语义，0=不采样）
	// > OTEL_TRACES_SAMPLER 环境变量 > SDK 默认 parentbased_always_on
	if s := o.sampler(); s != nil {
		tpOpts = append(tpOpts, sdktrace.WithSampler(s))
	}

	mu.Lock()
	provider = sdktrace.NewTracerProvider(tpOpts...)
	mu.Unlock()

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

// Shutdown 刷新并关闭 TracerProvider。幂等；Init 只执行一次，Shutdown 后不再重新初始化。
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
	samplerRatio *float64
}

// sampler 返回采样器。优先级：显式 samplerRatio > OTEL_TRACES_SAMPLER 环境变量 > nil（SDK 默认）。
func (o *options) sampler() sdktrace.Sampler {
	if o.samplerRatio != nil {
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(*o.samplerRatio))
	}
	return samplerFromEnv()
}

// samplerFromEnv 按 OTel 标准解析 OTEL_TRACES_SAMPLER / OTEL_TRACES_SAMPLER_ARG；
// 未设置或值无效时返回 nil。
func samplerFromEnv() sdktrace.Sampler {
	kind := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER"))
	if kind == "" {
		return nil
	}

	ratioSampler := func() sdktrace.Sampler {
		arg := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG"))
		r, err := strconv.ParseFloat(arg, 64)
		if err != nil || r < 0 || r > 1 {
			return nil
		}
		return sdktrace.TraceIDRatioBased(r)
	}

	switch kind {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return ratioSampler()
	case "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		if s := ratioSampler(); s != nil {
			return sdktrace.ParentBased(s)
		}
		return nil
	default:
		return nil
	}
}

type Protocol string

const (
	ProtocolGRPC Protocol = "grpc"
	ProtocolHTTP Protocol = "http"
)

// traceContextPropagator 固定使用 W3C TraceContext。
var traceContextPropagator = propagation.TraceContext{}

// newExporter 按 protocol 创建 OTLP exporter；未配置时默认 grpc。
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
