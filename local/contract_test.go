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
