package memory

// MemoryBackend names the storage strategy selected by the documented trigger.
// BackendFTS5 is a future migration target; this package still ships JSON only.
type MemoryBackend string

const (
	BackendJSON MemoryBackend = "json"
	BackendFTS5 MemoryBackend = "fts5"
)

type BackendMetrics struct {
	CorpusRecords  int
	RecallAt3      float64
	RecallMeasured bool
}

// SelectMemoryBackend is deliberately a pure policy decision. Unknown recall
// is not a failure measurement and therefore cannot trigger a dependency.
func SelectMemoryBackend(metrics BackendMetrics) MemoryBackend {
	if metrics.CorpusRecords > 500 || metrics.RecallMeasured && metrics.RecallAt3 < 0.70 {
		return BackendFTS5
	}
	return BackendJSON
}
