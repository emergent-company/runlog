package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	runlog "github.com/emergent-company/runlog"
)

type SSEEvent struct {
	Event string
	Data  string
}

type SSEBroker struct {
	mu        sync.Mutex
	clients   map[string][]chan SSEEvent
	startedAt time.Time
	db        *runlog.RunDB
	linterMgr *LinterManager
}

func newSSEBroker(db *runlog.RunDB) *SSEBroker {
	return &SSEBroker{
		clients:   make(map[string][]chan SSEEvent),
		startedAt: time.Now(),
		db:        db,
	}
}

func (b *SSEBroker) SetLinterManager(lm *LinterManager) {
	b.linterMgr = lm
}

func (b *SSEBroker) Subscribe(topic string) chan SSEEvent {
	ch := make(chan SSEEvent, 16)
	b.mu.Lock()
	b.clients[topic] = append(b.clients[topic], ch)
	b.mu.Unlock()
	return ch
}

func (b *SSEBroker) Unsubscribe(topic string, ch chan SSEEvent) {
	b.mu.Lock()
	subs := b.clients[topic]
	for i, c := range subs {
		if c == ch {
			b.clients[topic] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
	b.mu.Unlock()
}

func (b *SSEBroker) Publish(topic string, event SSEEvent) {
	b.mu.Lock()
	subs := b.clients[topic]
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (b *SSEBroker) Run(ctx context.Context) {
	go b.runFooterPoller(ctx)
}

func (b *SSEBroker) runFooterPoller(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			var totalRuns, totalTests int
			_ = b.db.RawDB().QueryRow(`SELECT COUNT(*) FROM test_runs`).Scan(&totalRuns)
			_ = b.db.RawDB().QueryRow(`SELECT COUNT(DISTINCT test_name) FROM test_runs`).Scan(&totalTests)

			uptime := time.Since(b.startedAt)
			uptimeStr := fmt.Sprintf("%.0fm", uptime.Minutes())
			if uptime.Hours() >= 1 {
				uptimeStr = fmt.Sprintf("%.0fh", uptime.Hours())
			}

			linterStatus := ""
			if b.linterMgr != nil {
				if running := b.linterMgr.Running(); len(running) > 0 {
					suffix := ""
					if len(running) > 1 {
						suffix = "s"
					}
					linterStatus = fmt.Sprintf(
						`<span class="loading loading-spinner loading-xs text-primary ml-2"></span><span class="text-primary ml-1">%d linter%s</span>`,
						len(running), suffix,
					)
				}
			}

			html := fmt.Sprintf(
				`<div class="status status-success status-xs"></div><span class="text-base-content/50">Running — %d runs, %d tests</span><span class="text-base-content/30 ml-2">up %s</span>%s`,
				totalRuns, totalTests, uptimeStr, linterStatus,
			)
			data, _ := json.Marshal(map[string]string{"html": html})
			b.Publish("footer", SSEEvent{Event: "footer-status", Data: string(data)})

		case <-ctx.Done():
			return
		}
	}
}
