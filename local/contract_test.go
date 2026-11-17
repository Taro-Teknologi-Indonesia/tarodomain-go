package local_test

import (
	"context"
	"testing"

	"github.com/Taro-Teknologi-Indonesia/tarodomain-go"
	"github.com/Taro-Teknologi-Indonesia/tarodomain-go/local"
	"github.com/Taro-Teknologi-Indonesia/tarodomain-go/tarotest"
)

func newProvider(context.Context) (tarodomain.Provider, error) { return local.NewProvider(), nil }

func TestEventStoreContract(t *testing.T) {
	tarotest.EventStoreContract(tarotest.Wrap(t), newProvider)
}

func TestSnapshotContract(t *testing.T) { tarotest.SnapshotContract(tarotest.Wrap(t), newProvider) }

func TestSagaStoreContract(t *testing.T) { tarotest.SagaStoreContract(tarotest.Wrap(t), newProvider) }

func TestProcessedEventsContract(t *testing.T) {
	tarotest.ProcessedEventsContract(tarotest.Wrap(t), newProvider)
}

func TestCommandIdempotencyContract(t *testing.T) {
	tarotest.CommandIdempotencyContract(tarotest.Wrap(t), newProvider)
}

func TestDeadLetterContract(t *testing.T) { tarotest.DeadLetterContract(tarotest.Wrap(t), newProvider) }

func TestUnitOfWorkContract(t *testing.T) { tarotest.UnitOfWorkContract(tarotest.Wrap(t), newProvider) }

func TestConcurrencyContract(t *testing.T) {
	tarotest.ConcurrencyContract(tarotest.Wrap(t), newProvider)
}
