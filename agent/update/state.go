package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	agentruntime "mosona-manager/agent/runtime"
)

type persistedState struct {
	ReleaseETag     string    `json:"release_etag,omitempty"`
	LastRemoteSHA   string    `json:"last_remote_sha,omitempty"`
	LastDownloadURL string    `json:"last_download_url,omitempty"`
	LastReleaseTag  string    `json:"last_release_tag,omitempty"`
	LastChecked     time.Time `json:"last_checked,omitempty"`
}

func statePath() string {
	return filepath.Join(agentruntime.InstallDir, "update.state.json")
}

func loadState() persistedState {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return persistedState{}
	}
	var s persistedState
	_ = json.Unmarshal(data, &s)
	return s
}

func saveState(s persistedState) {
	_ = os.MkdirAll(agentruntime.InstallDir, 0755)
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = os.WriteFile(statePath(), data, 0600)
}
