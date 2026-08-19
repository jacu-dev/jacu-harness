package memory

import "testing"

func TestSelectMemoryBackendHonorsDocumentedThresholds(t *testing.T) {
	tests := []struct {
		name    string
		metrics BackendMetrics
		want    MemoryBackend
	}{
		{name: "ordinary", metrics: BackendMetrics{CorpusRecords: 500, RecallAt3: 0.70, RecallMeasured: true}, want: BackendJSON},
		{name: "unknown recall does not fail open", metrics: BackendMetrics{CorpusRecords: 500}, want: BackendJSON},
		{name: "large corpus", metrics: BackendMetrics{CorpusRecords: 501, RecallAt3: 1, RecallMeasured: true}, want: BackendFTS5},
		{name: "low measured recall", metrics: BackendMetrics{CorpusRecords: 20, RecallAt3: 0.69, RecallMeasured: true}, want: BackendFTS5},
		{name: "boundary recall", metrics: BackendMetrics{CorpusRecords: 20, RecallAt3: 0.70, RecallMeasured: true}, want: BackendJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SelectMemoryBackend(tt.metrics); got != tt.want {
				t.Fatalf("backend = %q; want %q", got, tt.want)
			}
		})
	}
}
