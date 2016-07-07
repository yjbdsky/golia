package main

import (
	"time"

	"github.com/shirou/gopsutil/host"
)

type Collector struct {
	ch         chan Datapoint
	metricHead string
	interval   int
}

func (c *Collector) CollectAllMetric(metricNames []string) {
	for {
		for _, metricName := range metricNames {
			switch metricName {
			case "UpTimeAndProcs":
				go c.GetUpTimeAndProcs()
			}
		}
		time.Sleep(time.Second * time.Duration(c.interval))
	}
}

func (c *Collector) GetUpTimeAndProcs() {
	info, err := host.Info()
	if err == nil {
		log.Info("collecting up procs metrics")
		uptime := float64(info.Uptime)
		procs := float64(info.Procs)
		t := uint32(time.Now().Unix())
		c.ch <- Datapoint{c.metricHead + ".host.uptime", uptime, t}
		c.ch <- Datapoint{c.metricHead + ".host.procs", procs, t}
	}
}
