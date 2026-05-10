package memory_test

import (
	"testing"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
	"github.com/chaserensberger/wingman/store/storetest"
)

func TestMemoryConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) store.Store {
		return memory.NewStore()
	})
}
