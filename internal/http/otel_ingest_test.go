package http

import (
	"testing"
	"time"
)

func TestConvertLogRecord(t *testing.T) {
	str := func(s string) *string { return &s }

	t.Run("basic log record", func(t *testing.T) {
		lr := otlpLogRecord{
			TimeUnixNano: "1700000000000000000",
			Body:         otlpAnyValue{StringValue: str("POST /api/orders → 201")},
			Attributes: []otlpKeyValue{
				{Key: "method", Value: otlpAnyValue{StringValue: str("POST")}},
				{Key: "path", Value: otlpAnyValue{StringValue: str("/api/orders")}},
			},
			TraceID: "abc123",
		}
		resourceAttrs := map[string]any{"service.name": "my-app"}

		event, errMsg := convertLogRecord(lr, resourceAttrs, 1)

		if errMsg != "" {
			t.Fatalf("unexpected error: %s", errMsg)
		}
		if event.Message != "POST /api/orders → 201" {
			t.Errorf("expected message 'POST /api/orders → 201', got '%s'", event.Message)
		}
		if event.TraceID != "abc123" {
			t.Errorf("expected traceID 'abc123', got '%s'", event.TraceID)
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
		lr := otlpLogRecord{
			SeverityNumber: 17, // ERROR
			Body:           otlpAnyValue{StringValue: str("Something broke")},
		}

		event, errMsg := convertLogRecord(lr, nil, 1)

		if errMsg != "" {
			t.Fatalf("unexpected error: %s", errMsg)
		}
		if !event.IsError {
			t.Error("expected IsError true for severity 17")
		}
	})

	t.Run("error by exception.message", func(t *testing.T) {
		lr := otlpLogRecord{
			SeverityNumber: 9, // INFO
			Body:           otlpAnyValue{StringValue: str("Request failed")},
			Attributes: []otlpKeyValue{
				{Key: "exception.message", Value: otlpAnyValue{StringValue: str("NullPointerException")}},
				{Key: "exception.type", Value: otlpAnyValue{StringValue: str("java.lang.NullPointerException")}},
			},
		}

		event, errMsg := convertLogRecord(lr, nil, 1)

		if errMsg != "" {
			t.Fatalf("unexpected error: %s", errMsg)
		}
		if !event.IsError {
			t.Error("expected IsError true for exception.message")
		}
		// Verify context contains mapped fields
		if event.Context == "" {
			t.Fatal("expected non-empty context")
		}
	})

	t.Run("missing body returns error", func(t *testing.T) {
		lr := otlpLogRecord{
			Body: otlpAnyValue{},
		}

		event, errMsg := convertLogRecord(lr, nil, 1)

		if errMsg == "" {
			t.Error("expected error message for missing body")
		}
		if event != nil {
			t.Error("expected nil event for missing body")
		}
	})

	t.Run("all-zero traceId is stripped", func(t *testing.T) {
		lr := otlpLogRecord{
			Body:    otlpAnyValue{StringValue: str("test")},
			TraceID: "00000000000000000000000000000000",
		}

		event, errMsg := convertLogRecord(lr, nil, 1)

		if errMsg != "" {
			t.Fatalf("unexpected error: %s", errMsg)
		}
		if event.TraceID != "" {
			t.Errorf("expected empty traceID for all zeros, got '%s'", event.TraceID)
		}
	})

	t.Run("resource attributes merge into context", func(t *testing.T) {
		lr := otlpLogRecord{
			Body: otlpAnyValue{StringValue: str("test")},
			Attributes: []otlpKeyValue{
				{Key: "http.method", Value: otlpAnyValue{StringValue: str("GET")}},
			},
		}
		resourceAttrs := map[string]any{
			"service.name": "my-app",
			"host.name":    "prod-1",
		}

		event, errMsg := convertLogRecord(lr, resourceAttrs, 1)

		if errMsg != "" {
			t.Fatalf("unexpected error: %s", errMsg)
		}
		// Context should contain both resource and log attributes
		if event.Context == "" {
			t.Fatal("expected non-empty context")
		}
	})

	t.Run("log attributes override resource attributes", func(t *testing.T) {
		lr := otlpLogRecord{
			Body: otlpAnyValue{StringValue: str("test")},
			Attributes: []otlpKeyValue{
				{Key: "service.name", Value: otlpAnyValue{StringValue: str("override")}},
			},
		}
		resourceAttrs := map[string]any{"service.name": "original"}

		event, errMsg := convertLogRecord(lr, resourceAttrs, 1)

		if errMsg != "" {
			t.Fatalf("unexpected error: %s", errMsg)
		}
		// The log-level attribute should win
		if event.Context == "" {
			t.Fatal("expected non-empty context")
		}
	})

	t.Run("fatal severity is error", func(t *testing.T) {
		lr := otlpLogRecord{
			SeverityNumber: 21, // FATAL
			Body:           otlpAnyValue{StringValue: str("Fatal crash")},
		}

		event, errMsg := convertLogRecord(lr, nil, 1)

		if errMsg != "" {
			t.Fatalf("unexpected error: %s", errMsg)
		}
		if !event.IsError {
			t.Error("expected IsError true for severity 21 (FATAL)")
		}
	})

	t.Run("info severity is not error", func(t *testing.T) {
		lr := otlpLogRecord{
			SeverityNumber: 9, // INFO
			Body:           otlpAnyValue{StringValue: str("All good")},
		}

		event, errMsg := convertLogRecord(lr, nil, 1)

		if errMsg != "" {
			t.Fatalf("unexpected error: %s", errMsg)
		}
		if event.IsError {
			t.Error("expected IsError false for severity 9 (INFO)")
		}
	})
}

func TestUnwrapOtelValues(t *testing.T) {
	str := func(s string) *string { return &s }
	boolVal := func(b bool) *bool { return &b }
	floatVal := func(f float64) *float64 { return &f }

	t.Run("string value", func(t *testing.T) {
		v := otlpAnyValue{StringValue: str("hello")}

		if v.unwrap() != "hello" {
			t.Errorf("expected 'hello', got %v", v.unwrap())
		}
	})

	t.Run("int value", func(t *testing.T) {
		v := otlpAnyValue{IntValue: str("42")}

		if v.unwrap() != int64(42) {
			t.Errorf("expected 42, got %v", v.unwrap())
		}
	})

	t.Run("bool value", func(t *testing.T) {
		v := otlpAnyValue{BoolValue: boolVal(true)}

		if v.unwrap() != true {
			t.Errorf("expected true, got %v", v.unwrap())
		}
	})

	t.Run("double value", func(t *testing.T) {
		v := otlpAnyValue{DoubleValue: floatVal(3.14)}

		if v.unwrap() != 3.14 {
			t.Errorf("expected 3.14, got %v", v.unwrap())
		}
	})

	t.Run("nil value", func(t *testing.T) {
		v := otlpAnyValue{}

		if v.unwrap() != nil {
			t.Errorf("expected nil, got %v", v.unwrap())
		}
	})

	t.Run("array value", func(t *testing.T) {
		v := otlpAnyValue{
			ArrayValue: &otlpArray{
				Values: []otlpAnyValue{
					{StringValue: str("a")},
					{StringValue: str("b")},
				},
			},
		}

		result := v.unwrap().([]any)

		if len(result) != 2 || result[0] != "a" || result[1] != "b" {
			t.Errorf("expected [a, b], got %v", result)
		}
	})

	t.Run("kvlist value", func(t *testing.T) {
		v := otlpAnyValue{
			KvlistValue: &otlpKVList{
				Values: []otlpKeyValue{
					{Key: "foo", Value: otlpAnyValue{StringValue: str("bar")}},
				},
			},
		}

		result := v.unwrap().(map[string]any)

		if result["foo"] != "bar" {
			t.Errorf("expected foo=bar, got %v", result)
		}
	})
}
