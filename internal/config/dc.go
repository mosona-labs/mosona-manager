package config

import "sync"

var DynamicConf DynamicConfigType
var DConfLock = sync.RWMutex{}

func ReadDynamicConf() DynamicConfigType {
	DConfLock.RLock()
	defer DConfLock.RUnlock()
	return DynamicConf
}
