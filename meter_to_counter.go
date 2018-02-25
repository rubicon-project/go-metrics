package metrics

////////////////////////////////////////////////////////////////////////////
// Exposing meter functions/interfaces to replace with counter functionality
////////////////////////////////////////////////////////////////////////////

type Meter interface {
	Counter
}

func GetOrRegisterMeter(name string, r Registry) Meter {
	return GetOrRegisterCounter(name, r)
}

func NewMeter() Meter {
	return NewCounter()
}

func NewRegisteredMeter(name string, r Registry) Meter {
	return NewRegisteredCounter(name, r)
}

type MeterSnapshot struct {
	CounterSnapshot
}

type NilMeter struct {
	NilCounter
}

type StandardMeter struct {
	StandardCounter
}
