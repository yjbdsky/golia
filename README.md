#golia
通过psutil模块，抓取机器指标，发送到指定地址
conf/golia.ini :
ReloadInterval = 5
MetricInterval = 5
CarbonAddr= "XX.XX.XX.XX:2003"
MondoAddr  = "XX.XX.XX.XX:9518"
Metrics  = ["UpTimeAndProcs","Load","Misc","VirtualMemory","SwapMemory","CPU","NetIOCounters","DiskUsage","DiskIOCounters"]
LogLevel = "warning"
