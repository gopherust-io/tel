package tel

import (
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	samplerNever              = "never"
	samplerAlways             = "always"
	samplerTraceIDRatio       = "traceidratio"
	samplerStatusTraceIDRatio = "statustraceidratio"
	samplerParentBasedPrefix  = "parentbased_"
)

var errorAttributeKey = attribute.Key("error")

// StatusTraceIDRatioBased samples like TraceIDRatioBased, but always records when
// start attributes or links include an "error" key.
func StatusTraceIDRatioBased(fraction float64) sdktrace.Sampler {
	return statusTraceIDRatioSampler{
		inner:       sdktrace.TraceIDRatioBased(fraction),
		description: fmt.Sprintf("StatusTraceIDRatioBased{%g}", fraction),
	}
}

type statusTraceIDRatioSampler struct {
	inner       sdktrace.Sampler
	description string
}

func (s statusTraceIDRatioSampler) ShouldSample(params sdktrace.SamplingParameters) sdktrace.SamplingResult {
	res := s.inner.ShouldSample(params)
	if res.Decision == sdktrace.RecordAndSample {
		return res
	}
	if hasErrorAttr(params.Attributes) {
		res.Decision = sdktrace.RecordAndSample

		return res
	}
	for i := range params.Links {
		if hasErrorAttr(params.Links[i].Attributes) {
			res.Decision = sdktrace.RecordAndSample

			return res
		}
	}

	return res
}

func (s statusTraceIDRatioSampler) Description() string {
	return s.description
}

func hasErrorAttr(attrs []attribute.KeyValue) bool {
	for i := range attrs {
		if attrs[i].Key == errorAttributeKey {
			return true
		}
	}

	return false
}

// parseTraceSampler builds an SDK sampler from a config string.
// Supported: always | never | traceidratio:N | statustraceidratio:N |
// parentbased_<inner> (ParentBased wrapper).
func parseTraceSampler(spec string) sdktrace.Sampler {
	spec = strings.ToLower(strings.TrimSpace(spec))
	if spec == "" {
		spec = defaultTracesSampler
	}

	parentBased := false
	if strings.HasPrefix(spec, samplerParentBasedPrefix) {
		parentBased = true
		spec = strings.TrimPrefix(spec, samplerParentBasedPrefix)
	}

	inner := parseTraceSamplerInner(spec)
	if parentBased {
		return sdktrace.ParentBased(inner)
	}

	return inner
}

func parseTraceSamplerInner(spec string) sdktrace.Sampler {
	switch {
	case spec == samplerNever:
		return sdktrace.NeverSample()
	case spec == samplerAlways:
		return sdktrace.AlwaysSample()
	case strings.HasPrefix(spec, samplerStatusTraceIDRatio):
		return StatusTraceIDRatioBased(parseSamplerFraction(spec))
	case strings.HasPrefix(spec, samplerTraceIDRatio):
		return sdktrace.TraceIDRatioBased(parseSamplerFraction(spec))
	default:
		return StatusTraceIDRatioBased(0.1)
	}
}

func parseSamplerFraction(s string) float64 {
	_, rest, ok := strings.Cut(s, ":")
	if !ok {
		return 0
	}
	fraction, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
	if err != nil || fraction < 0 {
		return 0
	}
	if fraction > 1 {
		return 1
	}

	return fraction
}
