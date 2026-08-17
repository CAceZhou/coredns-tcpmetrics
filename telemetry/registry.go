package telemetry

import "sync"

var registry struct {
	sync.RWMutex
	store *Store
}

func SetDefaultStore(store *Store) { registry.Lock(); registry.store = store; registry.Unlock() }
func DefaultStore() *Store         { registry.RLock(); defer registry.RUnlock(); return registry.store }
func ClearDefaultStore(store *Store) {
	registry.Lock()
	defer registry.Unlock()
	if registry.store == store {
		registry.store = nil
	}
}
