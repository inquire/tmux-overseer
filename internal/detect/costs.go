package detect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CostEntry represents a single cost snapshot written to the daily ledger.
type CostEntry struct {
	SessionID string  `json:"session_id"`
	Source    string  `json:"source"`
	Name     string  `json:"name"`
	Cost     float64 `json:"cost"`
	Model    string  `json:"model,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// CostTracker maintains per-session high-water cost marks and writes
// incremental updates to a daily JSONL ledger file.
type CostTracker struct {
	mu       sync.Mutex
	highWater map[string]float64 // session_id -> last recorded cost
}

// NewCostTracker creates a new cost tracker.
func NewCostTracker() *CostTracker {
	return &CostTracker{
		highWater: make(map[string]float64),
	}
}

// ledgerPath returns the path to today's cost ledger file.
func ledgerPath() string {
	dir := statusDir()
	if dir == "" {
		return ""
	}
	today := time.Now().Format("2006-01-02")
	return filepath.Join(dir, "costs-"+today+".jsonl")
}

// RecordCost checks if the cost for a session has increased and appends
// to the daily ledger if so. Returns true if a new entry was written.
func (ct *CostTracker) RecordCost(sessionID, source, name, model string, cost float64) bool {
	if cost <= 0 {
		return false
	}

	ct.mu.Lock()
	defer ct.mu.Unlock()

	prev, ok := ct.highWater[sessionID]
	if ok && cost <= prev {
		return false
	}

	ct.highWater[sessionID] = cost

	entry := CostEntry{
		SessionID: sessionID,
		Source:    source,
		Name:     name,
		Cost:     cost,
		Model:    model,
		Timestamp: time.Now().Unix(),
	}

	path := ledgerPath()
	if path == "" {
		return false
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return false
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	_, _ = fmt.Fprintf(f, "%s\n", data)

	// Invalidate the DayCosts cache since we just wrote new data
	InvalidateDayCostsCache()

	return true
}

// TTL cache for DayCosts to avoid repeated ledger reads.
var dayCostsCache struct {
	mu         sync.Mutex
	perSession map[string]float64
	dayTotal   float64
	fetchedAt  time.Time
}

const dayCostsTTL = 5 * time.Second

// InvalidateDayCostsCache forces the next DayCosts() call to re-read the ledger.
func InvalidateDayCostsCache() {
	dayCostsCache.mu.Lock()
	dayCostsCache.fetchedAt = time.Time{}
	dayCostsCache.mu.Unlock()
}

// DayCosts reads today's ledger and returns the highest cost per session
// plus the total cost for the day. Results are cached with a 5-second TTL.
func DayCosts() (perSession map[string]float64, dayTotal float64) {
	dayCostsCache.mu.Lock()
	defer dayCostsCache.mu.Unlock()

	// Return cached result if still fresh
	if !dayCostsCache.fetchedAt.IsZero() && time.Since(dayCostsCache.fetchedAt) < dayCostsTTL {
		return dayCostsCache.perSession, dayCostsCache.dayTotal
	}

	perSession = make(map[string]float64)

	path := ledgerPath()
	if path == "" {
		dayCostsCache.perSession = perSession
		dayCostsCache.dayTotal = 0
		dayCostsCache.fetchedAt = time.Now()
		return
	}

	f, err := os.Open(path)
	if err != nil {
		dayCostsCache.perSession = perSession
		dayCostsCache.dayTotal = 0
		dayCostsCache.fetchedAt = time.Now()
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry CostEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Cost > perSession[entry.SessionID] {
			perSession[entry.SessionID] = entry.Cost
		}
	}

	for _, cost := range perSession {
		dayTotal += cost
	}

	// Update cache
	dayCostsCache.perSession = perSession
	dayCostsCache.dayTotal = dayTotal
	dayCostsCache.fetchedAt = time.Now()

	return
}

// SessionCost returns the persisted cost for a specific session from today's ledger.
func SessionCost(sessionID string) float64 {
	perSession, _ := DayCosts()
	return perSession[sessionID]
}
