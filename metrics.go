package main

import (
	"encoding/json"
	//	"fmt"
	//"fmt"
	"strings"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

//
type Collector struct {
	ch         chan Datapoint
	metricHead string
	interval   int
}

//
func (c *Collector) SendDatapoints(dps []Datapoint) {
	for _, dp := range dps {
		c.ch <- dp
	}
}

//
func (c *Collector) CollectAllMetric(metricNames []string) {
	for {
		for _, metricName := range metricNames {
			switch metricName {
			case "UpTimeAndProcs":
				go c.getUpTimeAndProcs()
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

func struct2map(v interface{}) (map[string]float64, error) {
	var data map[string]float64
	value, err := json.Marshal(v)
	if err != nil {
		return data, err
	}
	json.Unmarshal(value, &data)
	return data, nil
}
func map2dp(metricHead string, data map[string]float64) ([]Datapoint, error) {
	dps := []Datapoint{}
	t := uint32(time.Now().Unix())
	for name, value := range data {
		m := replaceSlash(metricHead + "." + name)
		dps = append(dps, Datapoint{m, value, t})
	}
	return dps, nil
}
func convert(metricHead string, v interface{}) ([]Datapoint, error) {
	dps := []Datapoint{}
	value, err := json.Marshal(v)
	if err != nil {
		return dps, err
	}
	var data map[string]float64
	json.Unmarshal(value, &data)
	//	if err1 != nil {
	//		return dps, err1
	//	}
	delete(data, "path")
	delete(data, "cpu")
	delete(data, "name")
	delete(data, "serialNumber")
	delete(data, "fstype")
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

func sanityCheck(k string, v float64) float64 {
	if v > 100.0 {
		log.Errorf("%s = %d is greater than  100\n", k, v)
		return 100.0
	}
	if v < 0.0 {
		log.Errorf("%s = %d is less than  0\n", k, v)
		return 0.0
	}
	return v
}

var last_cpu_jiffies map[string]float64
var last_total_jiffies float64

func cal_cpu(cpu_jiffies map[string]float64) map[string]float64 {
	var total float64
	data := make(map[string]float64)
	for _, v := range cpu_jiffies {
		total = total + v
	}
	if len(last_cpu_jiffies) == 0 {
		last_cpu_jiffies = make(map[string]float64)
		last_cpu_jiffies = cpu_jiffies
		last_total_jiffies = total
		return data
	}
	for k, v := range cpu_jiffies {
		tmp := (v - last_cpu_jiffies[k]) / (total - last_total_jiffies) * 100.0
		data[k] = sanityCheck(k, tmp)
	}
	last_total_jiffies = total
	last_cpu_jiffies = cpu_jiffies
	return data
}

func (c *Collector) getCPU() {
	v, err := cpu.Times(false)
	log.Info("collecting cpu.cpu_times metrics")
	cpu_jiffies, err := struct2map(v[0])
	if err == nil {
		delete(cpu_jiffies, "cpu")
		cpu_data := cal_cpu(cpu_jiffies)
		dps, err1 := map2dp(c.metricHead+".cpu.cpu_times", cpu_data)
		if err1 == nil && len(cpu_data) != 0 {
			c.SendDatapoints(dps)
		}
	}

}

func sum_netio(etl []net.IOCountersStat) (map[string]float64, error) {
	data := make(map[string]float64)
	index, err := struct2map(etl[0])
	if err != nil {
		return data, err
	}
	for k, _ := range index {
		for _, v := range etl {
			//Ignore 'lo' and 'bond*' interfaces
			//todo: add || !vlan
			if strings.HasPrefix(v.Name, "lo") || strings.HasPrefix(v.Name, "bond") {
				continue
			}
			tmp, _ := struct2map(v)
			data[k] = data[k] + tmp[k]
		}
	}
	return data, nil
}

var last_net_jiffies map[string]float64
var last_net_time uint32

func cal_net(net_jiffies map[string]float64) map[string]float64 {
	data := make(map[string]float64)
	t := uint32(time.Now().Unix())
	if len(last_net_jiffies) == 0 {
		last_net_jiffies = make(map[string]float64)
		last_net_jiffies = net_jiffies
		last_net_time = t
		return data
	}
	t_dur := t - last_net_time
	for k, v := range net_jiffies {
		data[k] = (v - last_net_jiffies[k]) / float64(t_dur)
	}
	last_net_jiffies = net_jiffies
	last_net_time = t
	return data
}

func (c *Collector) getNetIOCounters() {
	etl, err := net.IOCounters(true)
	if err != nil {
		log.Error(err)
		return
	}
	sumdata, err2 := sum_netio(etl)
	data := cal_net(sumdata)
	log.Info("collecting .net.IOCounters metrics")
	dps, err2 := map2dp(c.metricHead+".net.iocounters", data)
	if err2 == nil && len(data) != 0 {
		c.SendDatapoints(dps)
	}
}

func (c *Collector) getDiskUsage() {
	d, err := disk.Partitions(false)
	log.Info("collecting disk.usage metrics")
	if err == nil {
		for _, v := range d {
			u, err1 := disk.Usage(v.Mountpoint)
			if err1 == nil {
				dps, err2 := convert(c.metricHead+".disk.usage."+v.Mountpoint, u)
				if err2 == nil {
					c.SendDatapoints(dps)
				}
			}
		}
	}
}

func (c *Collector) getDiskIOCounters() {
	d, err := disk.IOCounters()
	log.Info("collecting disk.iocounters metrics")
	if err == nil {
		for name, v := range d {
			dps, err1 := convert(c.metricHead+".disk.IOCounters."+name, v)
			if err1 == nil {
				c.SendDatapoints(dps)
			}
		}
	}
}
