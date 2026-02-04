package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"time"
)

type LogEvent struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Component string            `json:"component"`
	DeviceID  string            `json:"deviceId"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
}

type Exporter struct {
	addr string
	ch   chan LogEvent
	mu   sync.Mutex
	conn net.Conn
}

func NewExporter(addr string) *Exporter {
	if addr == "" {
		return nil
	}
	addr = strings.TrimPrefix(addr, "tcp://")
	return &Exporter{
		addr: addr,
		ch:   make(chan LogEvent, 512),
	}
}

func (e *Exporter) Start(ctx context.Context) {
	if e == nil {
		return
	}
	go e.run(ctx)
}

func (e *Exporter) Send(ev LogEvent) {
	if e == nil {
		return
	}
	select {
	case e.ch <- ev:
	default:
		// Drop if backlog is full.
	}
}

func (e *Exporter) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			e.close()
			return
		case ev := <-e.ch:
			if err := e.write(ev); err != nil {
				e.close()
				// backoff before retrying
				timer := time.NewTimer(500 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				// requeue event if possible
				select {
				case e.ch <- ev:
				default:
				}
			}
		}
	}
}

func (e *Exporter) write(ev LogEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.conn == nil {
		conn, err := net.DialTimeout("tcp", e.addr, 2*time.Second)
		if err != nil {
			return err
		}
		e.conn = conn
	}

	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(e.conn)
	if _, err := w.Write(append(data, '\n')); err != nil {
		return err
	}
	return w.Flush()
}

func (e *Exporter) close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.conn != nil {
		_ = e.conn.Close()
		e.conn = nil
	}
}
