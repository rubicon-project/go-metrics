package metrics

import (
	"sync"
	"time"
)

// ThisMeters count events to produce exponentially-weighted moving average rates
// at one-, five-, and fifteen-minutes and a mean rate.
type ThisMeter interface {
	Count() int64
	Mark(int64)
	Rate1() float64
	Rate5() float64
	Rate15() float64
	RateMean() float64
	Snapshot() ThisMeter
	Stop()
}

// GetOrRegisterThisMeter returns an existing Meter or constructs and registers a
// new StandardThisMeter.
// Be sure to unregister the meter from the registry once it is of no use to
// allow for garbage collection.
func GetOrRegisterThisMeter(name string, r Registry) ThisMeter {
	if nil == r {
		r = DefaultRegistry
	}
	return r.GetOrRegister(name, NewThisMeter).(ThisMeter)
}

// NewThisMeter constructs a new StandardThisMeter and launches a goroutine.
// Be sure to call Stop() once the meter is of no use to allow for garbage collection.
func NewThisMeter() ThisMeter {
	if UseNilMetrics {
		return NilThisMeter{}
	}
	m := newStandardThisMeter()
	arbiter.Lock()
	defer arbiter.Unlock()
	arbiter.meters[m] = struct{}{}
	if !arbiter.started {
		arbiter.started = true
		go arbiter.tick()
	}
	return m
}

// NewRegisteredThisMeter constructs and registers a new StandardThisMeter and launches a
// goroutine.
// Be sure to unregister the meter from the registry once it is of no use to
// allow for garbage collection.
func NewRegisteredThisMeter(name string, r Registry) ThisMeter {
	c := NewThisMeter()
	if nil == r {
		r = DefaultRegistry
	}
	r.Register(name, c)
	return c
}

// ThisMeterSnapshot is a read-only copy of another Meter.
type ThisMeterSnapshot struct {
	count                          int64
	rate1, rate5, rate15, rateMean float64
}

// Count returns the count of events at the time the snapshot was taken.
func (m *ThisMeterSnapshot) Count() int64 { return m.count }

// Mark panics.
func (*ThisMeterSnapshot) Mark(n int64) {
	panic("Mark called on a ThisMeterSnapshot")
}

// Rate1 returns the one-minute moving average rate of events per second at the
// time the snapshot was taken.
func (m *ThisMeterSnapshot) Rate1() float64 { return m.rate1 }

// Rate5 returns the five-minute moving average rate of events per second at
// the time the snapshot was taken.
func (m *ThisMeterSnapshot) Rate5() float64 { return m.rate5 }

// Rate15 returns the fifteen-minute moving average rate of events per second
// at the time the snapshot was taken.
func (m *ThisMeterSnapshot) Rate15() float64 { return m.rate15 }

// RateMean returns the meter's mean rate of events per second at the time the
// snapshot was taken.
func (m *ThisMeterSnapshot) RateMean() float64 { return m.rateMean }

// Snapshot returns the snapshot.
func (m *ThisMeterSnapshot) Snapshot() ThisMeter { return m }

// Stop is a no-op.
func (m *ThisMeterSnapshot) Stop() {}

// NilThisMeter is a no-op Meter.
type NilThisMeter struct{}

// Count is a no-op.
func (NilThisMeter) Count() int64 { return 0 }

// Mark is a no-op.
func (NilThisMeter) Mark(n int64) {}

// Rate1 is a no-op.
func (NilThisMeter) Rate1() float64 { return 0.0 }

// Rate5 is a no-op.
func (NilThisMeter) Rate5() float64 { return 0.0 }

// Rate15is a no-op.
func (NilThisMeter) Rate15() float64 { return 0.0 }

// RateMean is a no-op.
func (NilThisMeter) RateMean() float64 { return 0.0 }

// Snapshot is a no-op.
func (NilThisMeter) Snapshot() ThisMeter { return NilThisMeter{} }

// Stop is a no-op.
func (NilThisMeter) Stop() {}

// StandardThisMeter is the standard implementation of a Meter.
type StandardThisMeter struct {
	lock        sync.RWMutex
	snapshot    *ThisMeterSnapshot
	a1, a5, a15 EWMA
	startTime   time.Time
	stopped     bool
}

func newStandardThisMeter() *StandardThisMeter {
	return &StandardThisMeter{
		snapshot:  &ThisMeterSnapshot{},
		a1:        NewEWMA1(),
		a5:        NewEWMA5(),
		a15:       NewEWMA15(),
		startTime: time.Now(),
	}
}

// Stop stops the meter, Mark() will be a no-op if you use it after being stopped.
func (m *StandardThisMeter) Stop() {
	m.lock.Lock()
	stopped := m.stopped
	m.stopped = true
	m.lock.Unlock()
	if !stopped {
		arbiter.Lock()
		delete(arbiter.meters, m)
		arbiter.Unlock()
	}
}

// Count returns the number of events recorded.
func (m *StandardThisMeter) Count() int64 {
	m.lock.RLock()
	count := m.snapshot.count
	m.lock.RUnlock()
	return count
}

// Mark records the occurance of n events.
func (m *StandardThisMeter) Mark(n int64) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.stopped {
		return
	}
	m.snapshot.count += n
	m.a1.Update(n)
	m.a5.Update(n)
	m.a15.Update(n)
	m.updateSnapshot()
}

// Rate1 returns the one-minute moving average rate of events per second.
func (m *StandardThisMeter) Rate1() float64 {
	m.lock.RLock()
	rate1 := m.snapshot.rate1
	m.lock.RUnlock()
	return rate1
}

// Rate5 returns the five-minute moving average rate of events per second.
func (m *StandardThisMeter) Rate5() float64 {
	m.lock.RLock()
	rate5 := m.snapshot.rate5
	m.lock.RUnlock()
	return rate5
}

// Rate15 returns the fifteen-minute moving average rate of events per second.
func (m *StandardThisMeter) Rate15() float64 {
	m.lock.RLock()
	rate15 := m.snapshot.rate15
	m.lock.RUnlock()
	return rate15
}

// RateMean returns the meter's mean rate of events per second.
func (m *StandardThisMeter) RateMean() float64 {
	m.lock.RLock()
	rateMean := m.snapshot.rateMean
	m.lock.RUnlock()
	return rateMean
}

// Snapshot returns a read-only copy of the meter.
func (m *StandardThisMeter) Snapshot() ThisMeter {
	m.lock.RLock()
	snapshot := *m.snapshot
	m.lock.RUnlock()
	return &snapshot
}

func (m *StandardThisMeter) updateSnapshot() {
	// should run with write lock held on m.lock
	snapshot := m.snapshot
	snapshot.rate1 = m.a1.Rate()
	snapshot.rate5 = m.a5.Rate()
	snapshot.rate15 = m.a15.Rate()
	snapshot.rateMean = float64(snapshot.count) / time.Since(m.startTime).Seconds()
}

func (m *StandardThisMeter) tick() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.a1.Tick()
	m.a5.Tick()
	m.a15.Tick()
	m.updateSnapshot()
}

// meterArbiter ticks meters every 5s from a single goroutine.
// meters are references in a set for future stopping.
type meterArbiter struct {
	sync.RWMutex
	started bool
	meters  map[*StandardThisMeter]struct{}
	ticker  *time.Ticker
}

var arbiter = meterArbiter{ticker: time.NewTicker(5e9), meters: make(map[*StandardThisMeter]struct{})}

// Ticks meters on the scheduled interval
func (ma *meterArbiter) tick() {
	for {
		select {
		case <-ma.ticker.C:
			ma.tickMeters()
		}
	}
}

func (ma *meterArbiter) tickMeters() {
	ma.RLock()
	defer ma.RUnlock()
	for meter := range ma.meters {
		meter.tick()
	}
}
