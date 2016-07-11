package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

type Collector struct {
	ch         chan Datapoint
	metricHead string
	interval   int
}

func (c *Collector) SendDatapoints(dps []Datapoint) {
	for dp := range dps {
		c.ch <- dp
	}
}

func (c *Collector) CollectAllMetric(metricNames []string) {
	for {
		for _, metricName := range metricNames {
			switch metricName {
			case "UpTimeAndProcs":
				go c.GetUpTimeAndProcs()
			case "Load":
				go c.getLoad()
			case "Misc":
				go c.getMisc()
			case "VirtualMemory":
				go c.getVirtualMemory()
			case "SwapMemory":
				go c.getSwapMemory()
			case "CPU":
				go c.getCPU()
			case "NetIOCounters":
				go c.getNetIOCounters()
			case "DiskUsage":
				go c.getDiskUsage()
			case "DiskIOCounters":
				go c.getDiskIOCounters()
			}
		}
		time.Sleep(time.Second * time.Duration(c.interval))
	}
}

func replaceSlash(input string) string {
	return strings.Replace(input, "/", "_", -1)
}

func convert(metricHead string, v interface{}) ([]Datapoint, error) {
	dps := []Datapoint{}
	value, err := json.Marshal(v)
	if err != nil {
		return dps, err
	}
	var data map[string]float64
	err1 := json.Unmarshal(value, &data)
	if err1 != nil {
		return dps, err1
	}
	t := uint32(time.Now().Unix())
	for name, value := range data {
		m := replaceSlash(metricHead + "." + name)
		dps = append(dps, Datapoint{m, value, t})
	}
	return dps, nil
}

func (c *Collector) getUpTimeAndProcs() {
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

func (c *Collector) getLoad() {
	la, _ := load.Avg()
	t := uint32(time.Now().Unix())
	log.Info("collecting load.loadx metrics")
	c.ch <- Datapoint{c.metricHead + ".load.load1", float64(la.Load1), t}
	c.ch <- Datapoint{c.metricHead + ".load.load5", float64(la.Load5), t}
	c.ch <- Datapoint{c.metricHead + ".load.load15", float64(la.Load15), t}
}

func (c *Collector) getMisc() {
	lm, _ := load.Misc()
	t := uint32(time.Now().Unix())
	log.Info("collecting Misc metrics")
	c.ch <- Datapoint{c.metricHead + ".Misc.procsRunning", float64(lm.ProcsRunning), t}
	c.ch <- Datapoint{c.metricHead + ".Misc.procsBlocked", float64(lm.ProcsBlocked), t}
	c.ch <- Datapoint{c.metricHead + ".Misc.ctxt", float64(lm.Ctxt), t}
}

func (c *Collector) getVirtualMemory() {
	v, err := mem.VirtualMemory()
	if err == nil {
		dps, err1 := convert(c.metricHead+".mem.memory", v)
		if err1 == nil {
			c.SendDatapoints(dps)
		}
	}
}

func (c *Collector) getSwapMemory() {
	v, err := mem.SwapMemory()
	if err == nil {
		dps, err1 := convert(c.metricHead+".mem.swap", v)
		if err1 == nil {
			c.SendDatapoints(dps)
		}
	}
}

func (c *Collector) getCPU() {
	v, err := cpu.Times(false)
	log.Info("collecting cpu.cpu_times metrics")
	if err == nil {
		dps, err1 := convert(c.metricHead+".cpu.cpu_times", v[0])
		if err1 == nil {
			c.SendDatapoints(dps)
		}
	}
}

func (c *Collector) getNetIOCounters() {
	v, err := net.IOCounters(false)
	log.Info("collecting .net.IOCounters metrics")
	if err == nil {
		dps, err1 := convert(c.metricHead+".net.iocounters", v[0])
		if err1 == nil {
			c.SendDatapoints(dps)
		}
	}
}

func (c *Clollector) getDiskUsage() {
	d, err := disk.Partitions(false)
	log.Info("collecting disk.usage metrics")
	if err == nil {
		for _, v := range d {
			u, err1 := disk.Usage(v.Mountpoint)
			if err1 == nil {
				dps, err1 := convert(c.metricHead+".disk.usage."+v.Mountpoint, u)
				c.SendDatapoints(dps)
			}
		}
	}
}

func (c *Collector) getDiskIOCounters() {
	d, err := disk.IOCounters()
	log.Info("collecting disk.iocounters metrics")
	if err == nil {
		for name, v := range d {
			dps, err1 := convert(c.metricHead+".net.IOCounters."+name, v)
			if err1 == nil {
				c.SendDatapoints(dps)
			}
		}
	}
}
