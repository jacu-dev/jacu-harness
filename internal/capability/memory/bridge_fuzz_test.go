package memory

import "testing"

func FuzzRenderAgentsBridgeNeverPanics(f *testing.F) {
	f.Add("Title", "Body")
	f.Add("<!-- JACU MEMORY END -->", "ghp_not-for-storage")
	f.Fuzz(func(t *testing.T, title, body string) {
		record := bridgeRecord("mem_0000000000000001", title, body)
		_, _, _ = RenderAgentsBridge([]Record{record})
	})
}
