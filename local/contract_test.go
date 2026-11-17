package local_test

import (
	"context"
	"testing"
	"time"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
	"github.com/Taro-Teknologi-Indonesia/tarodomain-go/local"
	"github.com/Taro-Teknologi-Indonesia/tarodomain-go/tarotest"
)

func newProvider(context.Context) (tarodomain.Provider, error) { return local.NewProvider(), nil }

func TestContract(t *testing.T) { tarotest.RunAll(tarotest.Wrap(t), newProvider) }

// A stopped clock gives every record the same timestamp: the suites must not
// depend on time moving.
func TestContractWithStoppedClock(t *testing.T) {
	tarotest.RunAll(tarotest.Wrap(t), func(context.Context) (tarodomain.Provider, error) {
		clock := tarodomain.NewManualClock(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
		return local.NewProviderWithDatabase(local.NewDatabase(clock)), nil
	})
}

// The recorder runs the suites without "go test", as the Cloudflare harness does.
func TestContractWithRecorder(t *testing.T) {
	r := tarotest.NewRecorder()
	tarotest.RunAll(r, newProvider)
	for _, f := range r.Failures() {
		t.Error(f)
	}
	if r.Cases() < 50 {
		t.Errorf("only %d cases ran", r.Cases())
	}
	if len(r.Skipped()) != 0 {
		t.Errorf("skipped: %v", r.Skipped())
	}
}
