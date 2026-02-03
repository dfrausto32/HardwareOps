package state

import (
	"encoding/json"
	"os"

	"github.com/google/uuid"
)

type State struct {
	DeviceID         string `json:"deviceId"`
	AgentVersion     string `json:"agentVersion"`
	CurrentVersion   string `json:"currentVersion"`
	CurrentConfigRev string `json:"currentConfigRev"`
	PreviousVersion  string `json:"previousVersion"`
	LastApplyStatus  string `json:"lastApplyStatus"`
	LastApplyError   string `json:"lastApplyError"`
}

func Load(path string) (State, error) {
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		var st State
		if err := json.NewDecoder(f).Decode(&st); err != nil {
			return State{}, err
		}
		return st, nil
	}

	st := State{DeviceID: uuid.NewString(), AgentVersion: "0.1.0"}
	if err := Save(path, st); err != nil {
		return State{}, err
	}
	return st, nil
}

func Save(path string, st State) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(st)
}
