package telemetry

import "testing"

func TestMeasuredBytesRequireMeasurement(t *testing.T) {
	input := validEventInput()
	input.OutputBytes = 42
	if _, err := NewEvent(input); err == nil {
		t.Fatal("output_bytes without measurement was accepted")
	}

	input.Measurement = "exact_bytes"
	event, err := NewEvent(input)
	if err != nil {
		t.Fatalf("measured event: %v", err)
	}
	if event.OutputBytes != 42 || event.Measurement != "exact_bytes" {
		t.Fatalf("measured event = %+v; want output_bytes 42 with exact_bytes", event)
	}
}
