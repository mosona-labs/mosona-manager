package config

import "sync"

var (
	dynamicConf     DynamicConfigType
	dynamicConfLock sync.RWMutex
)

func ReadDynamicConf() DynamicConfigType {
	dynamicConfLock.RLock()
	defer dynamicConfLock.RUnlock()
	return dynamicConf
}

func ReplaceDynamicConf(next DynamicConfigType) {
	dynamicConfLock.Lock()
	dynamicConf = next
	dynamicConfLock.Unlock()
}
