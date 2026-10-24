package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

func strVal(s string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
}

func attr(key, val string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: strVal(val)}
}

func TestConvertProtoLogRecord(t *testing.T) {
	t.Run("basic log record", func(t *testing.T) {
		lr := &logspb.LogRecord{
			TimeUnixNano: 1700000000000000000,
			Body:         strVal("POST /api/orders → 201"),
			Attributes: []*commonpb.KeyValue{
				attr("method", "POST"),
				attr("path", "/api/orders"),
			},
			TraceId: []byte{0xab, 0xc1, 0x23, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		}
		resourceAttrs := map[string]any{"service.name": "my-app"}

		event := convertProtoLogRecord(lr, resourceAttrs, 1)

		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if event.Message != "POST /api/orders → 201" {
			t.Errorf("expected message 'POST /api/orders → 201', got '%s'", event.Message)
		}
		if event.TraceID == "" {
			t.Error("expected non-empty traceID")
		}
		if event.IsError {
			t.Error("expected IsError false")
		}
		if event.APIKeyID != 1 {
			t.Errorf("expected APIKeyID 1, got %d", event.APIKeyID)
		}
		expected := time.Unix(0, 1700000000000000000)
		if !event.Timestamp.Equal(expected) {
			t.Errorf("expected timestamp %v, got %v", expected, event.Timestamp)
		}
	})

	t.Run("error by severity", func(t *testing.T) {
		lr := &logspb.LogRecord{
			SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
			Body:           strVal("Something broke"),
		}

		event := convertProtoLogRecord(lr, nil, 1)

		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if !event.IsError {
			t.Error("expected IsError true for ERROR severity")
		}
	})

	t.Run("error by exception.message", func(t *testing.T) {
		lr := &logspb.LogRecord{
			SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_INFO,
			Body:           strVal("Request failed"),
			Attributes: []*commonpb.KeyValue{
				attr("exception.message", "NullPointerException"),
				attr("exception.type", "java.lang.NullPointerException"),
			},
		}

		event := convertProtoLogRecord(lr, nil, 1)

		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if !event.IsError {
			t.Error("expected IsError true for exception.message")
		}
	})

	t.Run("keeps the exception's stack trace where the issue page reads it", func(t *testing.T) {
		lr := &logspb.LogRecord{
			SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
			Body:           strVal("Request failed"),
			Attributes: []*commonpb.KeyValue{
				attr("exception.type", "ValueError"),
				attr("exception.stacktrace", "Traceback (most recent call last):\n  File \"app.py\", line 3"),
			},
		}

		event := convertProtoLogRecord(lr, nil, 1)

		var ctx map[string]any
		json.Unmarshal([]byte(event.Context), &ctx)
		if trace, _ := ctx["stack_trace"].(string); !strings.HasPrefix(trace, "Traceback") {
			t.Errorf("stack_trace = %q, want the exception's stack trace", trace)
		}
	})

	t.Run("missing body returns nil", func(t *testing.T) {
		lr := &logspb.LogRecord{}

		event := convertProtoLogRecord(lr, nil, 1)

		if event != nil {
			t.Error("expected nil event for missing body")
		}
	})

	t.Run("all-zero traceId is stripped", func(t *testing.T) {
		lr := &logspb.LogRecord{
			Body:    strVal("test"),
			TraceId: make([]byte, 16), // all zeros
		}

		event := convertProtoLogRecord(lr, nil, 1)

		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if event.TraceID != "" {
			t.Errorf("expected empty traceID for all zeros, got '%s'", event.TraceID)
		}
	})

	t.Run("resource attributes merge into context", func(t *testing.T) {
		lr := &logspb.LogRecord{
			Body: strVal("test"),
			Attributes: []*commonpb.KeyValue{
				attr("http.method", "GET"),
			},
		}
		resourceAttrs := map[string]any{
			"service.name": "my-app",
			"host.name":    "prod-1",
		}

		event := convertProtoLogRecord(lr, resourceAttrs, 1)

		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if event.Context == "" {
			t.Fatal("expected non-empty context")
		}
	})

	t.Run("log attributes override resource attributes", func(t *testing.T) {
		lr := &logspb.LogRecord{
			Body: strVal("test"),
			Attributes: []*commonpb.KeyValue{
				attr("service.name", "override"),
			},
		}
		resourceAttrs := map[string]any{"service.name": "original"}

		event := convertProtoLogRecord(lr, resourceAttrs, 1)

		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if event.Context == "" {
			t.Fatal("expected non-empty context")
		}
	})

	t.Run("fatal severity is error", func(t *testing.T) {
		lr := &logspb.LogRecord{
			SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_FATAL,
			Body:           strVal("Fatal crash"),
		}

		event := convertProtoLogRecord(lr, nil, 1)

		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if !event.IsError {
			t.Error("expected IsError true for FATAL severity")
		}
	})

	t.Run("info severity is not error", func(t *testing.T) {
		lr := &logspb.LogRecord{
			SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_INFO,
			Body:           strVal("All good"),
		}

		event := convertProtoLogRecord(lr, nil, 1)

		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if event.IsError {
			t.Error("expected IsError false for INFO severity")
		}
	})

	t.Run("observed timestamp used when time_unix_nano is zero", func(t *testing.T) {
		lr := &logspb.LogRecord{
			TimeUnixNano:         0,
			ObservedTimeUnixNano: 1700000000000000000,
			Body:                 strVal("test"),
		}

		event := convertProtoLogRecord(lr, nil, 1)

		if event == nil {
			t.Fatal("expected event, got nil")
		}
		expected := time.Unix(0, 1700000000000000000)
		if !event.Timestamp.Equal(expected) {
			t.Errorf("expected observed timestamp %v, got %v", expected, event.Timestamp)
		}
	})
}

func TestUnwrapAnyValue(t *testing.T) {
	t.Run("string value", func(t *testing.T) {
		v := strVal("hello")

		if unwrapAnyValue(v) != "hello" {
			t.Errorf("expected 'hello', got %v", unwrapAnyValue(v))
		}
	})

	t.Run("int value", func(t *testing.T) {
		v := &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 42}}

		if unwrapAnyValue(v) != int64(42) {
			t.Errorf("expected 42, got %v", unwrapAnyValue(v))
		}
	})

	t.Run("bool value", func(t *testing.T) {
		v := &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}

		if unwrapAnyValue(v) != true {
			t.Errorf("expected true, got %v", unwrapAnyValue(v))
		}
	})

	t.Run("double value", func(t *testing.T) {
		v := &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 3.14}}

		if unwrapAnyValue(v) != 3.14 {
			t.Errorf("expected 3.14, got %v", unwrapAnyValue(v))
		}
	})

	t.Run("nil value", func(t *testing.T) {
		if unwrapAnyValue(nil) != nil {
			t.Errorf("expected nil, got %v", unwrapAnyValue(nil))
		}
	})

	t.Run("array value", func(t *testing.T) {
		v := &commonpb.AnyValue{
			Value: &commonpb.AnyValue_ArrayValue{
				ArrayValue: &commonpb.ArrayValue{
					Values: []*commonpb.AnyValue{strVal("a"), strVal("b")},
				},
			},
		}

		result := unwrapAnyValue(v).([]any)

		if len(result) != 2 || result[0] != "a" || result[1] != "b" {
			t.Errorf("expected [a, b], got %v", result)
		}
	})

	t.Run("kvlist value", func(t *testing.T) {
		v := &commonpb.AnyValue{
			Value: &commonpb.AnyValue_KvlistValue{
				KvlistValue: &commonpb.KeyValueList{
					Values: []*commonpb.KeyValue{
						{Key: "foo", Value: strVal("bar")},
					},
				},
			},
		}

		result := unwrapAnyValue(v).(map[string]any)

		if result["foo"] != "bar" {
			t.Errorf("expected foo=bar, got %v", result)
		}
	})
}
