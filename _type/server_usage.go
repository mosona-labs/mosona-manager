package _type

import "time"

type ServerUsageRecord struct {
	CPUUsage float64   `json:"cpu_usage"`
	Memory   float64   `json:"memory"`
	Time     time.Time `json:"time"`
}
