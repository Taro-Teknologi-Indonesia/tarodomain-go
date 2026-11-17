package tarodomain

import (
	"encoding/json"
	"testing"
	"time"
)

type bareProvider struct{ Provider }

type clockProvider struct {
	bareProvider
	c Clock
}

func (p clockProvider) Clock() Clock { return p.c }

type sleeperProvider struct {
	bareProvider
	s Sleeper
}

func (p sleeperProvider) Sleeper() Sleeper { return p.s }

func TestConsistencyModeText(t *testing.T) {
	for _, m := range []ConsistencyMode{ConsistencyStrong, ConsistencyEventual} {
		got, err := ParseConsistencyMode(m.String())
		if err != nil || got != m {
			t.Errorf("%s: %q, %v", m, got, err)
		}
		b, err := json.Marshal(EventBusConfig{Consistency: m, Transport: TransportInMemory})
		if err != nil {
			t.Fatal(err)
		}
		var back EventBusConfig
		if err := json.Unmarshal(b, &back); err != nil || back.Consistency != m {
			t.Errorf("json %s: %+v, %v", b, back, err)
		}
	}
	if _, err := ParseConsistencyMode("Strong"); !isCode(err, CodeInvalidConfiguration) {
		t.Errorf("bad text: %v", err)
	}
	var cfg EventBusConfig
	if err := json.Unmarshal([]byte(`{"Consistency":"sometimes"}`), &cfg); !isCode(err, CodeInvalidConfiguration) {
		t.Errorf("bad json: %v", err)
	}
	if _, err := json.Marshal(ConsistencyMode("")); err == nil {
		t.Error("empty mode marshalled")
	}
}

func TestProviderFallbacks(t *testing.T) {
	var p Provider = bareProvider{}
	if _, ok := ClockOf(p).(SystemClock); !ok {
		t.Errorf("clock = %T", ClockOf(p))
	}
	if _, ok := SleeperOf(p).(TimerSleeper); !ok {
		t.Errorf("sleeper = %T", SleeperOf(p))
	}

	mc := NewManualClock(time.Unix(1, 0))
	if ClockOf(clockProvider{c: mc}) != Clock(mc) {
		t.Error("provider clock ignored")
	}
	if _, ok := ClockOf(clockProvider{}).(SystemClock); !ok {
		t.Error("nil clock should fall back")
	}
	rs := &RecordingSleeper{}
	if SleeperOf(sleeperProvider{s: rs}) != Sleeper(rs) {
		t.Error("provider sleeper ignored")
	}
	if _, ok := SleeperOf(sleeperProvider{}).(TimerSleeper); !ok {
		t.Error("nil sleeper should fall back")
	}
}
