package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/lib/pq"

	"trove/backend/internal/data"
)

const notifyChannel = "trove_events"

// Dispatch worker pool. Run forwards every NOTIFY into a buffered channel and
// N workers dispatch in parallel — keeps pq's 32-deep Notify buffer from
// filling on slow DB reads. Cross-item ordering is lost, but each event
// carries (created_at,id) and the FE reducer is idempotent by cursor, so
// convergence holds. If the work queue fills (DB truly can't keep up) the
// hub broadcasts a single resync (rate-limited) so clients reload cleanly.
const (
	dispatchWorkers   = 4
	dispatchQueueSize = 512
	resyncCooldown    = 30 * time.Second
)

// Conn is one subscribed SSE client. send is buffered; if it fills (a client
// too slow to drain) the hub drops the connection — the client reconnects and
// catches up via cursor, so this is lossless by design, not data loss. The
// accessible project set is a snapshot taken at connect.
type Conn struct {
	userID   string
	projects map[string]struct{}
	send     chan Message
	done     chan struct{}
	once     sync.Once
}

func (c *Conn) close()                { c.once.Do(func() { close(c.done) }) }
func (c *Conn) C() <-chan Message     { return c.send }
func (c *Conn) Done() <-chan struct{} { return c.done }

// Hub owns the single pq.Listener and the connected-client registry. One per
// process; Run is the only goroutine that consumes notifications and fans
// them out.
type Hub struct {
	db    *sql.DB
	dsn   string
	mu    sync.RWMutex
	conns map[*Conn]struct{}
}

func NewHub(db *sql.DB, dsn string) *Hub {
	return &Hub{db: db, dsn: dsn, conns: map[*Conn]struct{}{}}
}

// Subscribe registers a client for its accessible project set (snapshot at
// connect). The caller must Unsubscribe when the stream ends.
func (h *Hub) Subscribe(userID string, projectIDs []string) *Conn {
	ps := make(map[string]struct{}, len(projectIDs))
	for _, id := range projectIDs {
		ps[id] = struct{}{}
	}
	c := &Conn{
		userID:   userID,
		projects: ps,
		send:     make(chan Message, 64),
		done:     make(chan struct{}),
	}
	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) Unsubscribe(c *Conn) {
	h.mu.Lock()
	delete(h.conns, c)
	h.mu.Unlock()
	c.close()
}

// Run consumes pg NOTIFYs and fans them out until ctx is cancelled. On cancel
// it closes every connection so in-flight SSE handlers return and graceful
// shutdown completes. On a Listener reconnect it broadcasts `resync` (events
// during the DB→hub gap won't have tripped a client-side reconnect, so clients
// must re-run cursor catch-up).
func (h *Hub) Run(ctx context.Context) {
	lev := make(chan pq.ListenerEventType, 4)
	l := pq.NewListener(h.dsn, 2*time.Second, time.Minute, func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("sse listener: %v", err)
		}
		select {
		case lev <- ev:
		default:
		}
	})
	defer l.Close()

	// Listen returning an error with gotResponse=true (server-side rejection)
	// does NOT add the channel to pq's reconnect registry — so initial
	// failures are permanent. Retry a few times with linear backoff, then
	// crash so Fly restarts us rather than serving a silently-broken hub.
	for i := 0; i < 5; i++ {
		err := l.Listen(notifyChannel)
		if err == nil {
			break
		}
		if i == 4 {
			log.Fatalf("sse listen %q: %v", notifyChannel, err)
		}
		log.Printf("sse listen %q: retry %d: %v", notifyChannel, i, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(5*(i+1)) * time.Second):
		}
	}

	ping := time.NewTicker(90 * time.Second)
	defer ping.Stop()

	// Workers: closed-chan signals shutdown; ctx cancellation also unblocks
	// any in-flight DB reads inside dispatch, so wg.Wait returns promptly.
	work := make(chan string, dispatchQueueSize)
	var wg sync.WaitGroup
	for i := 0; i < dispatchWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range work {
				h.dispatch(ctx, p)
			}
		}()
	}
	defer func() {
		close(work)
		wg.Wait()
	}()

	var lastResync time.Time

	for {
		select {
		case <-ctx.Done():
			h.closeAll()
			return
		case ev := <-lev:
			if ev == pq.ListenerEventReconnected {
				h.broadcast(Message{Event: EventResync})
			}
		case <-ping.C:
			go func() { _ = l.Ping() }()
		case n := <-l.Notify:
			if n == nil { // pq sends a nil on reconnect — ignore
				continue
			}
			select {
			case work <- n.Extra:
			default:
				// Queue full → drop the NOTIFY and broadcast resync so
				// clients reload (rate-limited to avoid thundering herd).
				if time.Since(lastResync) > resyncCooldown {
					log.Printf("sse dispatch backlog full — broadcasting resync")
					h.broadcast(Message{Event: EventResync})
					lastResync = time.Now()
				}
			}
		}
	}
}

func (h *Hub) dispatch(ctx context.Context, payload string) {
	var env Envelope
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		log.Printf("sse envelope: %v", err)
		return
	}
	// Skip the DB read entirely if no connected client can see this project.
	if !h.anyInterested(env.ProjectID) {
		return
	}
	a, err := data.GetActivityByID(ctx, h.db, env.ActivityID)
	if err != nil {
		log.Printf("sse get activity %s: %v", env.ActivityID, err)
		return
	}
	msgs, err := Build(ctx, h.db, a)
	if err != nil {
		log.Printf("sse build %s: %v", env.ActivityID, err)
		return
	}
	for _, m := range msgs {
		h.fanOut(m)
	}
}

func (h *Hub) anyInterested(projectID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.conns {
		if _, ok := c.projects[projectID]; ok {
			return true
		}
	}
	return false
}

// targets snapshots the conns matching projectID (or all, if projectID == "").
func (h *Hub) targets(projectID string) []*Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Conn, 0, len(h.conns))
	for c := range h.conns {
		if projectID == "" {
			out = append(out, c)
			continue
		}
		if _, ok := c.projects[projectID]; ok {
			out = append(out, c)
		}
	}
	return out
}

func (h *Hub) deliver(targets []*Conn, m Message) {
	for _, c := range targets {
		select {
		case c.send <- m:
		default:
			// Slow client: drop it. It reconnects + catches up via cursor.
			h.Unsubscribe(c)
		}
	}
}

func (h *Hub) fanOut(m Message)    { h.deliver(h.targets(m.ProjectID), m) }
func (h *Hub) broadcast(m Message) { h.deliver(h.targets(""), m) }

func (h *Hub) closeAll() {
	h.mu.Lock()
	for c := range h.conns {
		c.close()
	}
	h.conns = map[*Conn]struct{}{}
	h.mu.Unlock()
}
