package otel

import (
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"github.com/stretchr/testify/require"
)

func TestSamplerFromEnv(t *testing.T) {
	decision := func(s sdktrace.Sampler) (sdktrace.SamplingDecision, bool) {
		if s == nil {
			return 0, false
		}
		return s.ShouldSample(sdktrace.SamplingParameters{
			TraceID: trace.TraceID{1},
		}).Decision, true
	}

	tests := []struct {
		name    string
		sampler string
		arg     string
		wantNil bool
		want    sdktrace.SamplingDecision
	}{
		{"unset", "", "", true, 0},
		{"always_on", "always_on", "", false, sdktrace.RecordAndSample},
		{"always_off", "always_off", "", false, sdktrace.Drop},
		{"parentbased_always_on", "parentbased_always_on", "", false, sdktrace.RecordAndSample},
		{"parentbased_always_off", "parentbased_always_off", "", false, sdktrace.Drop},
		{"traceidratio zero", "traceidratio", "0", false, sdktrace.Drop},
		{"traceidratio one", "traceidratio", "1", false, sdktrace.RecordAndSample},
		{"parentbased_traceidratio zero", "parentbased_traceidratio", "0", false, sdktrace.Drop},
		{"parentbased_traceidratio one", "parentbased_traceidratio", "1", false, sdktrace.RecordAndSample},
		{"traceidratio invalid arg", "traceidratio", "abc", true, 0},
		{"traceidratio missing arg", "traceidratio", "", true, 0},
		{"traceidratio arg out of range", "traceidratio", "2", true, 0},
		{"unknown value", "unknown", "", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_SAMPLER", tt.sampler)
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.arg)
			got, ok := decision(samplerFromEnv())
			if tt.wantNil {
				require.False(t, ok)
				return
			}
			require.True(t, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestOptions_Sampler_ExplicitRatioZeroDisablesSampling(t *testing.T) {
	ratio := 0.0
	o := &options{samplerRatio: &ratio}
	require.Equal(t, sdktrace.Drop, o.sampler().ShouldSample(sdktrace.SamplingParameters{
		TraceID: trace.TraceID{1},
	}).Decision)
}

func TestOptions_Sampler_UnsetFallsBackToEnv(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")
	o := &options{}
	require.Equal(t, sdktrace.Drop, o.sampler().ShouldSample(sdktrace.SamplingParameters{
		TraceID: trace.TraceID{1},
	}).Decision)
}

func TestOptions_Sampler_UnsetWithoutEnvUsesSDKDefault(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "")
	o := &options{}
	require.Nil(t, o.sampler(), "no explicit sampler means SDK default")
}
