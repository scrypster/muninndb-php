package replication

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/internal/transport/mbp"
	"github.com/vmihailenco/msgpack/v5"
)

type blockingWriteConn struct {
	firstWriteStarted chan struct{}
	releaseFirstWrite chan struct{}
	secondWrite       chan struct{}
	closed            chan struct{}
}

func newBlockingWriteConn() *blockingWriteConn {
	return &blockingWriteConn{
		firstWriteStarted: make(chan struct{}),
		releaseFirstWrite: make(chan struct{}),
		secondWrite:       make(chan struct{}, 1),
		closed:            make(chan struct{}),
	}
}

func (c *blockingWriteConn) Read(_ []byte) (int, error)         { <-c.closed; return 0, io.EOF }
func (c *blockingWriteConn) LocalAddr() net.Addr                { return dummyAddr("local") }
func (c *blockingWriteConn) RemoteAddr() net.Addr               { return dummyAddr("remote") }
func (c *blockingWriteConn) SetDeadline(_ time.Time) error      { return nil }
func (c *blockingWriteConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *blockingWriteConn) SetWriteDeadline(_ time.Time) error { return nil }
func (c *blockingWriteConn) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return nil
}

func (c *blockingWriteConn) Write(p []byte) (int, error) {
	select {
	case <-c.firstWriteStarted:
		select {
		case c.secondWrite <- struct{}{}:
		default:
		}
		<-c.closed
		return 0, net.ErrClosed
	default:
		close(c.firstWriteStarted)
		<-c.releaseFirstWrite
		select {
		case <-c.closed:
			return 0, net.ErrClosed
		default:
			return len(p), nil
		}
	}
}

type dummyAddr string

func (a dummyAddr) Network() string { return "test" }
func (a dummyAddr) String() string  { return string(a) }

// newTestLog opens a temporary Pebble database and returns a ReplicationLog
// backed by it. The caller owns cleanup via t.Cleanup.
func newTestLog(t *testing.T) *ReplicationLog {
	t.Helper()
	dir := t.TempDir()
	db, err := pebble.Open(dir, &pebble.Options{})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewReplicationLog(db)
}

// pipeConn creates a pair of in-process net.Pipe connections and wraps the
// client side in a PeerConn (no TCP dial required).
func pipeConn(t *testing.T) (pc *PeerConn, server net.Conn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		serverConn.Close()
		clientConn.Close()
	})
	pc = &PeerConn{
		nodeID: "test-peer",
		addr:   "pipe",
		conn:   clientConn,
	}
	return pc, serverConn
}

// readReplEntry reads one MBP frame from conn and unmarshals the ReplEntry payload.
func readReplEntry(t *testing.T, conn net.Conn) mbp.ReplEntry {
	t.Helper()
	frame, err := mbp.ReadFrame(conn)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if frame.Type != mbp.TypeReplEntry {
		t.Fatalf("frame type = %d, want TypeReplEntry (%d)", frame.Type, mbp.TypeReplEntry)
	}
	var entry mbp.ReplEntry
	if err := msgpack.Unmarshal(frame.Payload, &entry); err != nil {
		t.Fatalf("msgpack.Unmarshal ReplEntry: %v", err)
	}
	return entry
}

// TestNetworkStreamer_SendsEntries verifies that writing 3 entries to the log
// causes the NetworkStreamer to send all 3 over the peer connection.
func TestNetworkStreamer_SendsEntries(t *testing.T) {
	log := newTestLog(t)
	pc, server := pipeConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ns := NewNetworkStreamer(log, pc, 0)
	streamErr := make(chan error, 1)
	go func() { streamErr <- ns.Stream(ctx) }()

	// Write 3 entries.
	for i := 0; i < 3; i++ {
		if _, err := log.Append(OpSet, []byte("key"), []byte("value")); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	// Read 3 frames from the server side.
	for i := 0; i < 3; i++ {
		entry := readReplEntry(t, server)
		if entry.Seq != uint64(i+1) {
			t.Errorf("entry %d: seq = %d, want %d", i, entry.Seq, i+1)
		}
	}

	cancel()
	<-streamErr
}

// TestNetworkStreamer_PushLatency verifies that NetworkStreamer delivers an
// appended entry far below the old 100ms poll interval.
//
// Total end-to-end time includes Pebble's fsync (pebble.Sync) which dominates
// on local storage. What matters is that the push mechanism adds negligible
// overhead on top of fsync — so we bound at 50ms to avoid being flaky while
// still proving we are nowhere near 100ms.
func TestNetworkStreamer_PushLatency(t *testing.T) {
	log := newTestLog(t)
	pc, server := pipeConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ns := NewNetworkStreamer(log, pc, 0)
	streamErr := make(chan error, 1)
	go func() { streamErr <- ns.Stream(ctx) }()

	// Append one entry and measure how long until the frame arrives.
	start := time.Now()
	if _, err := log.Append(OpSet, []byte("latency-key"), []byte("latency-val")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Set a deadline so ReadFrame doesn't block indefinitely on failure.
	server.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	readReplEntry(t, server)
	elapsed := time.Since(start)

	// 50ms is well below the old 100ms poll floor and leaves headroom for
	// fsync on slow CI disks.
	if elapsed >= 50*time.Millisecond {
		t.Errorf("push latency = %v, want < 50ms (old polling floor was 100ms)", elapsed)
	}
	t.Logf("push latency = %v (includes Pebble fsync)", elapsed)

	cancel()
	if err := <-streamErr; err != nil && err != context.Canceled {
		t.Fatalf("Stream returned %v, want nil/context.Canceled", err)
	}
}

// TestNetworkStreamer_ResumeFromSeq verifies that starting a streamer at
// startSeq=5 causes only entries 6-8 to be streamed when entries 1-8 exist.
func TestNetworkStreamer_ResumeFromSeq(t *testing.T) {
	log := newTestLog(t)

	// Pre-write entries 1-5 before the streamer starts.
	for i := 0; i < 5; i++ {
		if _, err := log.Append(OpSet, []byte("pre"), []byte("v")); err != nil {
			t.Fatalf("pre-Append %d: %v", i, err)
		}
	}

	pc, server := pipeConn(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ns := NewNetworkStreamer(log, pc, 5) // start after seq 5
	streamErr := make(chan error, 1)
	go func() { streamErr <- ns.Stream(ctx) }()

	// Write entries 6, 7, 8.
	for i := 0; i < 3; i++ {
		if _, err := log.Append(OpSet, []byte("new"), []byte("v")); err != nil {
			t.Fatalf("new Append %d: %v", i, err)
		}
	}

	// Should receive exactly entries 6, 7, 8.
	for want := uint64(6); want <= 8; want++ {
		entry := readReplEntry(t, server)
		if entry.Seq != want {
			t.Errorf("seq = %d, want %d", entry.Seq, want)
		}
	}

	cancel()
	<-streamErr
}

// TestNetworkStreamer_CatchesUpExistingEntries verifies that the network
// streamer sends entries that were already present in the replication log when
// the streamer starts, without requiring a fresh append notification.
func TestNetworkStreamer_CatchesUpExistingEntries(t *testing.T) {
	log := newTestLog(t)

	for i := 0; i < 3; i++ {
		if _, err := log.Append(OpSet, []byte("existing"), []byte("v")); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	pc, server := pipeConn(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ns := NewNetworkStreamer(log, pc, 0)
	streamErr := make(chan error, 1)
	go func() { streamErr <- ns.Stream(ctx) }()

	for want := uint64(1); want <= 3; want++ {
		entry := readReplEntry(t, server)
		if entry.Seq != want {
			t.Errorf("seq = %d, want %d", entry.Seq, want)
		}
	}

	cancel()
	if err := <-streamErr; err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		t.Fatalf("Stream returned unexpected error: %v", err)
	}
}

// TestNetworkStreamer_ContextCancel verifies that Stream() returns promptly
// when the context is cancelled.
func TestNetworkStreamer_ContextCancel(t *testing.T) {
	log := newTestLog(t)
	pc, _ := pipeConn(t)

	ctx, cancel := context.WithCancel(context.Background())

	ns := NewNetworkStreamer(log, pc, 0)
	streamErr := make(chan error, 1)
	go func() { streamErr <- ns.Stream(ctx) }()

	cancel()

	select {
	case err := <-streamErr:
		if err != context.Canceled {
			t.Errorf("Stream returned %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Stream did not return within 500ms after cancel")
	}
}

// TestNetworkStreamer_CancelDuringCatchup verifies that cancellation stops the
// initial backlog drain before a second send is attempted.
func TestNetworkStreamer_CancelDuringCatchup(t *testing.T) {
	log := newTestLog(t)
	for i := 0; i < 3; i++ {
		if _, err := log.Append(OpSet, []byte("existing"), []byte("v")); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	conn := newBlockingWriteConn()
	pc := &PeerConn{nodeID: "test-peer", addr: "blocking", conn: conn}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ns := NewNetworkStreamer(log, pc, 0)
	streamErr := make(chan error, 1)
	go func() { streamErr <- ns.Stream(ctx) }()

	select {
	case <-conn.firstWriteStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first catchup send did not start")
	}

	cancel()
	close(conn.releaseFirstWrite)

	select {
	case err := <-streamErr:
		if err != context.Canceled {
			t.Fatalf("Stream returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not return after cancel during catchup")
	}

	select {
	case <-conn.secondWrite:
		t.Fatal("stream attempted a second backlog send after cancellation")
	default:
	}
}

// TestReplicationLog_Subscribe_Notification verifies that appending an entry
// causes the subscriber channel to receive a signal within 1ms.
func TestReplicationLog_Subscribe_Notification(t *testing.T) {
	log := newTestLog(t)

	notify, unsub := log.Subscribe()
	defer unsub()

	if _, err := log.Append(OpSet, []byte("k"), []byte("v")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	select {
	case <-notify:
		// received
	case <-time.After(1 * time.Millisecond):
		t.Error("subscriber not notified within 1ms")
	}
}

// TestReplicationLog_Subscribe_MultipleSubscribers verifies that a single
// append notifies all registered subscribers.
func TestReplicationLog_Subscribe_MultipleSubscribers(t *testing.T) {
	log := newTestLog(t)

	const n = 3
	notifies := make([]<-chan struct{}, n)
	unsubs := make([]func(), n)
	for i := 0; i < n; i++ {
		notifies[i], unsubs[i] = log.Subscribe()
		defer unsubs[i]()
	}

	if _, err := log.Append(OpSet, []byte("k"), []byte("v")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	for i := 0; i < n; i++ {
		select {
		case <-notifies[i]:
			// received
		case <-time.After(5 * time.Millisecond):
			t.Errorf("subscriber %d not notified within 5ms", i)
		}
	}
}

// TestReplicationLog_Unsubscribe verifies that after calling unsubscribe,
// further appends do not send on the now-closed channel.
func TestReplicationLog_Unsubscribe(t *testing.T) {
	log := newTestLog(t)

	notify, unsub := log.Subscribe()

	// First append — should notify.
	if _, err := log.Append(OpSet, []byte("k"), []byte("v")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	select {
	case <-notify:
	case <-time.After(5 * time.Millisecond):
		t.Fatal("expected notification before unsubscribe")
	}

	// Unsubscribe — channel is closed by unsub().
	unsub()

	// Drain any pending signal (there shouldn't be one, but be safe).
	select {
	case <-notify:
	default:
	}

	// Second append — must not panic (send on closed channel would panic).
	if _, err := log.Append(OpSet, []byte("k2"), []byte("v2")); err != nil {
		t.Fatalf("Append after unsub: %v", err)
	}

	// Give a moment to confirm no send happened.
	select {
	case _, ok := <-notify:
		if ok {
			t.Error("received notification after unsubscribe")
		}
		// ok==false means channel was closed by unsub — that's expected.
	case <-time.After(5 * time.Millisecond):
		// No send — correct.
	}
}
