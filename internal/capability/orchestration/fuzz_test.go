package orchestration

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
)

func FuzzFlowSpecNeverPanicsOrExecutesBlockedGraph(f *testing.F) {
	f.Add([]byte(`{"flow":{"nodes":[{"id":"a","uses":"review"}],"edges":[]}}`))
	f.Add([]byte(`{"flow":{"nodes":[{"id":"verify","uses":"verify"},{"id":"apply","uses":"apply"}],"edges":[{"from":"verify","to":"apply","when":"verdict == not_run"}]}}`))
	f.Add([]byte(`{"flow":{"nodes":[{"id":"a","uses":"review","max_visits":2},{"id":"b","uses":"review"}],"edges":[{"from":"a","to":"b","when":"default"},{"from":"b","to":"a","when":"default"}]}}`))
	f.Fuzz(func(t *testing.T, payload []byte) {
		var input Input
		if json.Unmarshal(payload, &input) != nil {
			return
		}
		validation := Validate(input.Flow)
		var called atomic.Bool
		_, _ = Execute(context.Background(), input.Flow, input.Context, func(context.Context, Node, map[string]NodeResult) (NodeResult, error) {
			called.Store(true)
			return NodeResult{Status: "ok", Verdict: "pass"}, nil
		})
		if validation.Blocked() && called.Load() {
			t.Fatal("blocked graph executed a node")
		}
	})
}
