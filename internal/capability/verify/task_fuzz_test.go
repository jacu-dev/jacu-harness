package verify

import (
	"encoding/json"
	"testing"
)

func FuzzValidTaskIDNeverPanics(f *testing.F) {
	f.Add("task_1111111111111111")
	f.Add("../task")
	f.Add("")
	f.Fuzz(func(_ *testing.T, value string) {
		_ = ValidTaskID(value)
	})
}

func FuzzTaskInputDecodeNeverPanics(f *testing.F) {
	f.Add([]byte(`{"run_id":"run_1111111111111111","async":true}`))
	f.Add([]byte{0xff, 0x00, '{'})
	f.Fuzz(func(_ *testing.T, payload []byte) {
		var input Input
		_ = json.Unmarshal(payload, &input)
		_ = ValidTaskID(input.TaskID)
	})
}
