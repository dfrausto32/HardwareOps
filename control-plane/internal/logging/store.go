package logging

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) Append(ev LogEvent) error {
	if s == nil {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}

	filename := "unknown.csv"
	if ev.DeviceID != "" {
		filename = "device-" + ev.DeviceID + ".csv"
	}
	path := filepath.Join(s.dir, filename)

	s.mu.Lock()
	defer s.mu.Unlock()

	needsHeader := false
	if info, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			needsHeader = true
		} else {
			return err
		}
	} else if info.Size() == 0 {
		needsHeader = true
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if needsHeader {
		if err := w.Write([]string{"timestamp", "level", "component", "deviceId", "message", "fields"}); err != nil {
			return err
		}
	}
	fields := ""
	if len(ev.Fields) > 0 {
		if b, err := json.Marshal(ev.Fields); err == nil {
			fields = string(b)
		}
	}
	if err := w.Write([]string{
		ev.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		ev.Level,
		ev.Component,
		ev.DeviceID,
		ev.Message,
		fields,
	}); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}
