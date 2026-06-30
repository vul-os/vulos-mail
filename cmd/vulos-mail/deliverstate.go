package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/vul-os/vulos-mail/services/mtaout"
)

// deliverStatePersister snapshots and restores the outbound deliverability state
// (per-domain warmup ramp + per-tenant reputation counters) to a JSON file so it
// survives restarts. This state is advisory — losing it never loses mail — but
// resetting it on every redeploy would reset warmup ramps to day 0 and clear
// reputation gates, both of which hurt deliverability / abuse containment.
//
// Concurrency: load() runs before the scheduler loop starts, and save() is
// invoked only from that same scheduler-loop goroutine between Ticks, so the
// (lock-free) Warmup/Reputation snapshots never race a Tick mutation.
type deliverStatePersister struct {
	path string
	warm *mtaout.Warmup
	rep  *mtaout.Reputation
}

type deliverStateFile struct {
	Warmup     mtaout.WarmupState             `json:"warmup"`
	Reputation map[string]mtaout.RepStatState `json:"reputation"`
}

func newDeliverStatePersister(path string, warm *mtaout.Warmup, rep *mtaout.Reputation) *deliverStatePersister {
	return &deliverStatePersister{path: path, warm: warm, rep: rep}
}

func (p *deliverStatePersister) load() {
	data, err := os.ReadFile(p.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("mtaout state: load %s: %v (starting fresh)", p.path, err)
		}
		return
	}
	var st deliverStateFile
	if err := json.Unmarshal(data, &st); err != nil {
		log.Printf("mtaout state: parse %s: %v (starting fresh)", p.path, err)
		return
	}
	if p.warm != nil {
		p.warm.Restore(st.Warmup)
	}
	if p.rep != nil && st.Reputation != nil {
		p.rep.Restore(st.Reputation)
	}
	log.Printf("mtaout state: restored warmup/reputation from %s", p.path)
}

func (p *deliverStatePersister) save() {
	st := deliverStateFile{}
	if p.warm != nil {
		st.Warmup = p.warm.Snapshot()
	}
	if p.rep != nil {
		st.Reputation = p.rep.Snapshot()
	}
	data, err := json.Marshal(st)
	if err != nil {
		log.Printf("mtaout state: marshal: %v", err)
		return
	}
	tmp := p.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		log.Printf("mtaout state: mkdir: %v", err)
		return
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Printf("mtaout state: write: %v", err)
		return
	}
	if err := os.Rename(tmp, p.path); err != nil {
		log.Printf("mtaout state: rename: %v", err)
	}
}
