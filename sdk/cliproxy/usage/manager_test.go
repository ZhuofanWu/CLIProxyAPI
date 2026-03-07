package usage

import (
	"context"
	"sync"
	"testing"
	"time"
)

type blockingPlugin struct {
	mu      sync.Mutex
	count   int
	started chan struct{}
	release chan struct{}
}

func (p *blockingPlugin) HandleUsage(ctx context.Context, record Record) {
	if p.started != nil {
		close(p.started)
		p.started = nil
	}
	if p.release != nil {
		<-p.release
	}
	p.mu.Lock()
	p.count++
	p.mu.Unlock()
}

func (p *blockingPlugin) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

func TestManagerStopWaitsForDrain(t *testing.T) {
	mgr := NewManager(8)
	plugin := &blockingPlugin{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	mgr.Register(plugin)
	mgr.Start(context.Background())
	mgr.Publish(context.Background(), Record{Provider: "test"})

	select {
	case <-plugin.started:
	case <-time.After(2 * time.Second):
		t.Fatal("plugin did not start processing")
	}

	stopped := make(chan struct{})
	go func() {
		mgr.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("Stop returned before pending work drained")
	case <-time.After(100 * time.Millisecond):
	}

	close(plugin.release)

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not wait for worker completion")
	}

	if plugin.Count() != 1 {
		t.Fatalf("processed records = %d, want 1", plugin.Count())
	}
}
