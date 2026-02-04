package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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

type Ingestor struct {
	addr  string
	store *Store
	logf  func(format string, args ...any)
	wg    sync.WaitGroup
}

func StartIngest(ctx context.Context, addr string, store *Store, logf func(string, ...any)) (*Ingestor, error) {
	if addr == "" || store == nil {
		return nil, errors.New("log ingest addr and store required")
	}
	addr = strings.TrimPrefix(addr, "tcp://")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	ing := &Ingestor{addr: addr, store: store, logf: logf}
	ing.wg.Add(1)
	go func() {
		defer ing.wg.Done()
		defer ln.Close()
		if logf != nil {
			logf("log ingest listening on %s", addr)
		}
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if logf != nil {
					logf("log ingest accept error: %v", err)
				}
				continue
			}
			ing.wg.Add(1)
			go func(c net.Conn) {
				defer ing.wg.Done()
				defer c.Close()
				scanner := bufio.NewScanner(c)
				for scanner.Scan() {
					var ev LogEvent
					if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
						if logf != nil {
							logf("log ingest decode error: %v", err)
						}
						continue
					}
					if err := store.Append(ev); err != nil && logf != nil {
						logf("log ingest store error: %v", err)
					}
				}
			}(conn)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	return ing, nil
}

func (i *Ingestor) Wait() {
	if i == nil {
		return
	}
	i.wg.Wait()
}
