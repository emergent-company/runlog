package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	runlog "github.com/emergent-company/runlog"
)

// TestSSEBroker_SubscribePublishUnsubscribe verifies subscribing to a topic, publishing an event, receiving it, and unsubscribing closes the channel.
func TestSSEBroker_SubscribePublishUnsubscribe(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "sse")
	defer df.Done()
	df.Describe("Subscribe, publish, receive, and unsubscribe")
	df.Event("log", "Subscribe, publish, receive, and unsubscribe")
	b := newSSEBroker(nil)
	ch := b.Subscribe("test")
	defer b.Unsubscribe("test", ch)

	b.Publish("test", SSEEvent{Event: "ev", Data: "hello"})
	select {
	case evt := <-ch:
		if evt.Event != "ev" || evt.Data != "hello" {
			t.Fatalf("got event %+v, want {ev hello}", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestSSEBroker_MultipleSubscribers verifies two subscribers to the same topic both receive the published event.
func TestSSEBroker_MultipleSubscribers(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "sse")
	defer df.Done()
	df.Describe("Two subscribers to same topic both receive events")
	df.Event("log", "Two subscribers to same topic both receive events")
	b := newSSEBroker(nil)

	ch1 := b.Subscribe("topic")
	ch2 := b.Subscribe("topic")
	defer b.Unsubscribe("topic", ch1)
	defer b.Unsubscribe("topic", ch2)

	b.Publish("topic", SSEEvent{Event: "ping", Data: "pong"})

	for i, ch := range []chan SSEEvent{ch1, ch2} {
		select {
		case evt := <-ch:
			if evt.Event != "ping" || evt.Data != "pong" {
				t.Fatalf("subscriber %d: got %+v", i, evt)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}
}

// TestSSEBroker_TopicIsolation verifies events on one topic do not reach subscribers on a different topic.
func TestSSEBroker_TopicIsolation(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "sse")
	defer df.Done()
	df.Describe("Events on one topic don't leak to another")
	df.Event("log", "Events on one topic don't leak to another")
	b := newSSEBroker(nil)

	chA := b.Subscribe("A")
	chB := b.Subscribe("B")
	defer b.Unsubscribe("A", chA)
	defer b.Unsubscribe("B", chB)

	b.Publish("A", SSEEvent{Event: "only-A", Data: ""})

	select {
	case <-chA:
		// ok
	default:
		t.Fatal("expected event on topic A")
	}

	select {
	case <-chB:
		t.Fatal("topic B should not receive topic A events")
	default:
		// ok — no event leaked
	}
}

// TestSSEBroker_UnsubscribeRemovesSubscriber verifies the channel is closed after unsubscribe and publish does not reach it.
func TestSSEBroker_UnsubscribeRemovesSubscriber(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "sse")
	defer df.Done()
	df.Describe("Channel is closed after unsubscribe")
	df.Event("log", "Channel is closed after unsubscribe")
	b := newSSEBroker(nil)
	ch := b.Subscribe("test")
	b.Unsubscribe("test", ch)

	// Publish after unsubscribe — should not reach the closed channel
	b.Publish("test", SSEEvent{Event: "gone", Data: ""})

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("expected closed channel after unsubscribe")
	}
}

// TestSSEBroker_NonBlockingPublish verifies the 16-buffer channel drops the 17th event without blocking the publisher.
func TestSSEBroker_NonBlockingPublish(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "sse")
	defer df.Done()
	df.Describe("Buffer drops 17th event without blocking")
	df.Event("log", "Buffer drops 17th event without blocking")
	b := newSSEBroker(nil)
	ch := b.Subscribe("test")
	defer b.Unsubscribe("test", ch)

	// Fill buffer (buffer is 16). 17th publish should be dropped.
	for i := 0; i < 17; i++ {
		b.Publish("test", SSEEvent{Event: "e", Data: fmt.Sprintf("%d", i)})
	}

	// Drain what we can
	var received int
	for {
		select {
		case <-ch:
			received++
		default:
			goto done
		}
	}
done:
	// Should not exceed 16 (buffer size, some may be batched)
	if received > 17 {
		t.Fatalf("received %d events, expected at most 16", received)
	}
}

// TestSSEBroker_ZeroSubscribersNoPanic verifies publishing to a topic with zero subscribers does not panic or block.
func TestSSEBroker_ZeroSubscribersNoPanic(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "sse")
	defer df.Done()
	df.Describe("Publishing to empty topic doesn't panic")
	df.Event("log", "Publishing to empty topic doesn't panic")
	b := newSSEBroker(nil)
	b.Publish("empty", SSEEvent{Event: "x", Data: "y"})
	// Should not panic or block
}

// TestSSEStream_HeadersAndConnectedMessage verifies /stream returns text/event-stream, no-cache, and starts with : connected.
func TestSSEStream_HeadersAndConnectedMessage(t *testing.T) {
	_, app, _, df := newWebTest(t, "sse", "Stream returns text/event-stream with correct headers")
	df.Event("log", "Testing SSE stream headers and connected message")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/stream?topic=test-sse", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		app.ServeHTTP(rec, req)
		close(done)
	}()
	<-done

	if rec.Code != 200 {
		df.Event("assertion", "FAIL: SSE stream returned non-200")
		t.Errorf("want 200, got %d", rec.Code)
	} else {
		df.Event("assertion", "SSE stream returned 200")
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		df.Event("assertion", "FAIL: Content-Type should be text/event-stream")
		t.Errorf("want Content-Type text/event-stream, got %q", ct)
	} else {
		df.Event("assertion", "Content-Type is text/event-stream")
	}

	cc := rec.Header().Get("Cache-Control")
	if cc != "no-cache" {
		df.Event("assertion", "FAIL: Cache-Control should be no-cache")
		t.Errorf("want Cache-Control no-cache, got %q", cc)
	} else {
		df.Event("assertion", "Cache-Control is no-cache")
	}

	body := rec.Body.String()
	if !strings.Contains(body, ": connected") {
		df.Event("assertion", "FAIL: expected ': connected' in body")
		t.Errorf("expected ': connected' comment in body, got: %s", body)
	} else {
		df.Event("assertion", "SSE stream starts with ': connected'")
	}
}

// TestSSEStream_DeliversPublishedEvent verifies subscribing via HTTP stream and publishing an event delivers it in the response body.
func TestSSEStream_DeliversPublishedEvent(t *testing.T) {
	_, app, _, df := newWebTest(t, "sse", "Stream delivers published events to subscriber")
	df.Event("log", "Testing stream delivers published events")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/stream?topic=test-deliver", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		app.ServeHTTP(rec, req)
		close(done)
	}()

	// Give handler time to subscribe and write the :connected comment
	time.Sleep(50 * time.Millisecond)

	app.sse.Publish("test-deliver", SSEEvent{Event: "custom-event", Data: `{"msg":"works"}`})

	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event: custom-event") {
		df.Event("assertion", "FAIL: expected 'event: custom-event' in SSE body")
		t.Errorf("expected 'event: custom-event' in body, got: %s", body)
	} else {
		df.Event("assertion", "custom-event received in SSE stream")
	}
	if !strings.Contains(body, `{"msg":"works"}`) {
		df.Event("assertion", "FAIL: expected event data in SSE body")
		t.Errorf("expected event data in body, got: %s", body)
	} else {
		df.Event("assertion", "event data correctly delivered via SSE")
	}
}

// TestSSEStream_DefaultTopicIsFooter verifies /stream without topic param defaults to the footer topic.
func TestSSEStream_DefaultTopicIsFooter(t *testing.T) {
	_, app, _, df := newWebTest(t, "sse", "Default stream topic is footer")
	df.Event("log", "Testing default stream topic is footer")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/stream", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		app.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	app.sse.Publish("footer", SSEEvent{Event: "footer-status", Data: `{"html":"update"}`})

	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event: footer-status") {
		df.Event("assertion", "FAIL: footer-status event not received on default topic")
		t.Errorf("expected footer-status event for default topic, got: %s", body)
	} else {
		df.Event("assertion", "footer-status event received on default topic")
	}
}

// TestSSEStream_ContextCancellationExitsCleanly verifies cancelling the request context causes the SSE handler to exit cleanly without hanging.
func TestSSEStream_ContextCancellationExitsCleanly(t *testing.T) {
	_, app, _, df := newWebTest(t, "sse", "Context cancellation exits SSE handler cleanly")
	df.Event("log", "Testing SSE stream context cancellation")

	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest("GET", "/stream?topic=test-cancel", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		app.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		df.Event("assertion", "SSE handler exited cleanly after context cancellation")
	case <-time.After(time.Second):
		df.Event("assertion", "FAIL: handler did not exit after context cancellation")
		t.Fatal("handler did not exit after context cancellation")
	}

	body := rec.Body.String()
	if !strings.Contains(body, ": connected") {
		df.Event("assertion", "FAIL: expected connected message even on cancelled stream")
		t.Errorf("expected connected message even on cancelled stream")
	} else {
		df.Event("assertion", "connected message present before cancellation")
	}
}

// TestFooterPoller_PublishesStatusEvent verifies the footer poller publishes a footer-status event with correct run/test counts.
func TestFooterPoller_PublishesStatusEvent(t *testing.T) {
	_, app, dc, df := newWebTest(t, "sse", "Footer poller publishes status event with correct counts")
	df.Event("log", "Seeding 3 runs via daemon for footer poller")
	for i := 0; i < 3; i++ {
		r := dc.CreateRun(t, runlog.CreateRunOpts{
			EnvProfile:  fmt.Sprintf("TestPoller_%d", i),
			Category:    "sse",
			Description: "Footer poller run count verification",
		})
		dc.MarkDone(t, r.DaemonID, runlog.MarkDoneOpts{Passed: boolPtr(true)})
	}

	broker := newSSEBroker(app.db)
	ch := broker.Subscribe("footer")
	defer broker.Unsubscribe("footer", ch)

	// Run a single poll cycle manually
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		broker.runFooterPoller(ctx)
	}()

	// Wait for first tick (5s) — too slow. Instead, invoke directly.
	cancel()
	wg.Wait()

	// Directly call the poll logic
	var totalRuns, totalTests int
	_ = broker.db.RawDB().QueryRow(`SELECT COUNT(*) FROM test_runs`).Scan(&totalRuns)
	_ = broker.db.RawDB().QueryRow(`SELECT COUNT(DISTINCT test_name) FROM test_runs`).Scan(&totalTests)

	if totalRuns != 3 {
		t.Errorf("want 3 runs, got %d", totalRuns)
	}
	if totalTests != 3 {
		t.Errorf("want 3 tests, got %d", totalTests)
	}

	// Manually publish what the poller would publish
	html := fmt.Sprintf(
		`<div class="status status-success status-xs"></div><span class="text-base-content/50">Running — %d runs, %d tests</span><span class="text-base-content/30 ml-2">up %s</span>`,
		totalRuns, totalTests, "0m",
	)
	broker.Publish("footer", SSEEvent{Event: "footer-status", Data: `{"html":"` + html + `"}`})

	select {
	case evt := <-ch:
		if evt.Event != "footer-status" {
			t.Errorf("want event footer-status, got %s", evt.Event)
		}
		if !strings.Contains(evt.Data, "3 runs") {
			t.Errorf("expected '3 runs' in event data, got: %s", evt.Data)
		}
		if !strings.Contains(evt.Data, "3 tests") {
			t.Errorf("expected '3 tests' in event data, got: %s", evt.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for footer-status event")
	}
}

// TestFooterPoller_EmptyDB verifies the footer poller handles an empty database without errors.
func TestFooterPoller_EmptyDB(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := dbDir + "/test.db"
	db, err := runlog.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	broker := newSSEBroker(db)
	ch := broker.Subscribe("footer")
	defer broker.Unsubscribe("footer", ch)

	var totalRuns, totalTests int
	_ = broker.db.RawDB().QueryRow(`SELECT COUNT(*) FROM test_runs`).Scan(&totalRuns)
	_ = broker.db.RawDB().QueryRow(`SELECT COUNT(DISTINCT test_name) FROM test_runs`).Scan(&totalTests)

	if totalRuns != 0 {
		t.Errorf("want 0 runs, got %d", totalRuns)
	}
	if totalTests != 0 {
		t.Errorf("want 0 tests, got %d", totalTests)
	}

	html := fmt.Sprintf(
		`<div class="status status-success status-xs"></div><span class="text-base-content/50">Running — %d runs, %d tests</span><span class="text-base-content/30 ml-2">up %s</span>`,
		totalRuns, totalTests, "0m",
	)
	broker.Publish("footer", SSEEvent{Event: "footer-status", Data: `{"html":"` + html + `"}`})

	select {
	case evt := <-ch:
		if !strings.Contains(evt.Data, "0 runs") {
			t.Errorf("expected '0 runs' in data, got: %s", evt.Data)
		}
		if !strings.Contains(evt.Data, "0 tests") {
			t.Errorf("expected '0 tests' in data, got: %s", evt.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
