package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	logging "github.com/op/go-logging"
)

const (
	SIGINT  = syscall.SIGINT
	SIGQUIT = syscall.SIGQUIT
	SIGTERM = syscall.SIGTERM
)

type Config struct {
	ReloadInterval    int
	MetricInterval    int
	CarbonAddr        string
	MondoAddr         string
	Metrics           []string
	LogLevel          string
	HeartbeatInterval int
	HeartbeatUrl      string
	GoliaconfUrl      string
}

func Init() {
	var format = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`)
	//logFile,err =  os.OpenFile("log/golia.log", os.O_WRONLY,0644)
	var logBackend = logging.NewLogBackend(os.Stderr, "", 0)
	logging.SetFormatter(format)
	logging.SetBackend(logBackend)

}

var log = logging.MustGetLogger("golia")
var ch = make(chan Datapoint)
var conn *Conn
var conf Config

func collectAndSend(addr string, interval int) {
	var err error
	conn, err = NewConn(addr)
	if err == nil && conn != nil {
		for {
			dp := <-ch
			if !conn.isAlive() {
				log.Warningf("**********failed to reconnect, sleep %d sec", interval)
				time.Sleep(time.Second * time.Duration(interval))
				os.Exit(3)
			}
			n, err1 := conn.WriteDataPoint(dp)
			if err1 != nil || n == 0 {
				log.Warningf("can't send metric %v", dp)
			}
		}
	} else {
		log.Error(err)
		log.Warningf("**********sleep %d sec", interval)
		time.Sleep(time.Second * time.Duration(interval))
		os.Exit(3)
	}
}

func lookPath() (argv0 string, err error) {
	argv0, err = exec.LookPath(os.Args[0])
	if nil != err {
		return
	}
	if _, err = os.Stat(argv0); nil != err {
		return
	}
	return
}

func reloaderLoop(path string, surl string, interval int) {
	var md5 string
	for {
		tag, err := getMd5(path)
		sdata, err1 := getUrlconf(surl)
		if err1 == nil {
			stag, err1 := getByteMd5(sdata)
			CheckErr(err1)
			if stag != tag && err1 == nil {
				log.Warning("detect change form URL....", stag, tag)
				ioutil.WriteFile(path, sdata, 0755)
				tag, err = getMd5(path)
			}
		} else {
			log.Warning("[ERROR]:", err1)
		}

		if err != nil {
			log.Error(err)
			os.Exit(4)
		}
		if md5 == "" {
			md5 = tag
		} else if tag != md5 {
			log.Info("detect change ....")
			os.Exit(3)
		}
		time.Sleep(time.Second * time.Duration(interval))
	}
}
func restartWithReloader() int {
	for {
		os.Setenv("RUN_MAIN", "true")
		argv0, _ := lookPath()
		files := make([]*os.File, 3)
		files[0] = os.Stdin
		files[1] = os.Stdout
		files[2] = os.Stderr
		wd, _ := os.Getwd()
		p, err := os.StartProcess(argv0, os.Args, &os.ProcAttr{Dir: wd, Env: os.Environ(), Files: files, Sys: &syscall.SysProcAttr{}})
		if err != nil {
			return 2
		}
		ret, err := p.Wait()
		if err != nil {
			return 2
		}
		ws, ok := ret.Sys().(syscall.WaitStatus)
		if !ok {
			return 2
		}
		if code := ws.ExitStatus(); code != 3 {
			return code
		}
	}
}

func handleExit(sigs chan os.Signal) {
	sig := <-sigs
	log.Infof("receive signal exiting...%v\n", sig)
	close(ch)
	if conn == nil {
		os.Exit(2)
	}
	if conn.isAlive() {
		err := conn.Close()
		if err == nil {
			log.Infof("exited...%v\n", sig)
			os.Exit(0)
		} else {
			log.Infof("exited error...%v\n", err)
			os.Exit(2)
		}
	}
}

func getMd5(path string) (ret string, err error) {
	file, err := os.Open(path)
	defer file.Close()
	if err == nil {
		md5h := md5.New()
		io.Copy(md5h, file)
		ret = fmt.Sprintf("%x", md5h.Sum([]byte("")))
	}
	return
}

func getByteMd5(s []byte) (string, error) {
	md5h := md5.New()
	_, err := md5h.Write(s)
	ret := fmt.Sprintf("%x", md5h.Sum(nil))
	return ret, err
}

func getAddrByDefault() (string, error) {
	conn1, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}
	defer conn1.Close()
	return strings.Split(conn1.LocalAddr().String(), ":")[0], nil
}

func GetAddr(addr string) (r string, err error) {
	resp, err := http.Get(addr)
	if err == nil {
		defer resp.Body.Close()
		body, err1 := ioutil.ReadAll(resp.Body)
		if err1 == nil {
			r = string(body)
			err = err1
			return
		}
	}
	r, err = getAddrByDefault()
	return
}

func writePidFile(path string) {
	if path == "" {
		path = "run/golia.pid"
	}
	if pid := syscall.Getpid(); pid != 1 {
		ioutil.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
	}
}

func sepurl(surl string) string {
	if !strings.HasPrefix(surl, "http://") {
		surl = "http://" + surl
	}
	if strings.Contains(surl, "$MondoAddr") {
		surl = strings.Replace(surl, "$MondoAddr", conf.MondoAddr, -1)
	}
	return surl
}

var c *http.Client = &http.Client{
	Transport: &http.Transport{
		Dial: func(netw, addr string) (net.Conn, error) {
			deadline := time.Now().Add(25 * time.Second)
			c, err := net.DialTimeout(netw, addr, time.Second*20)
			if err != nil {
				return nil, err
			}
			c.SetDeadline(deadline)
			return c, nil
		},
	},
}

func heartbeat(surl string, interval int) {
	surl = sepurl(surl)
	for {
		res, err := c.Get(surl)
		//defer res.Body.Close()
		if err != nil {
			log.Warning("heartbeat failed [ERROR]:", err)
			time.Sleep(time.Second * time.Duration(interval))
			continue
		}
		body, err := ioutil.ReadAll(res.Body)
		if res.StatusCode != 200 && string(body) != "ok" {
			if string(body) == "notreg" {
				purl := string([]byte(surl)[:strings.LastIndex(surl, "/")]) + "/reg"
				log.Warning(purl)
				reg(purl)
			} else {
				log.Warningf("heartbeat failed StatusCode is %d\n", res.StatusCode)
			}

		} else {
			log.Info("heartbeat sucess")
		}

		time.Sleep(time.Second * time.Duration(interval))
	}
}
func CheckErr(err error) {
	if err != nil {
		log.Warning("[ERROR]:", err)
	}
}

func getUrlconf(surl string) (data []byte, err error) {
	surl = sepurl(surl)
	res, err := c.Get(surl)
	if err != nil {
		log.Warning("get failed [ERROR]:", err)
		return []byte(""), err
	}
	data, err = ioutil.ReadAll(res.Body)
	return data, err
}

func main() {
	flag.Parse()
	config_file := "conf/golia.ini"
	if 1 == flag.NArg() {
		config_file = flag.Arg(0)
	}
	if _, err := toml.DecodeFile(config_file, &conf); err != nil {
		log.Error("config file error")
		return
	}
	levels := map[string]logging.Level{
		"critical": logging.CRITICAL,
		"error":    logging.ERROR,
		"warning":  logging.WARNING,
		"notice":   logging.NOTICE,
		"info":     logging.INFO,
		"debug":    logging.DEBUG,
	}
	level, ok := levels[conf.LogLevel]
	if !ok {
		log.Error("unrecognized log level '%s'\n", conf.LogLevel)
		return
	}
	logging.SetLevel(level, "golia")

	if os.Getenv("RUN_MAIN") == "true" {
		log.Info("golia subprocess start")
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		ip, err := GetAddr(conf.MondoAddr)
		if err != nil {
			os.Exit(1)
		}
		log.Infof("using ip address %s\n", ip)
		metricHead := "golia." + strings.Replace(ip, ".", "_", -1)
		collector := Collector{ch, metricHead, conf.MetricInterval}
		go handleExit(sigs)
		go collectAndSend(conf.CarbonAddr, conf.MetricInterval)
		go collector.CollectAllMetric(conf.Metrics)
		go heartbeat(conf.HeartbeatUrl, conf.HeartbeatInterval)
		reloaderLoop(config_file, conf.GoliaconfUrl, conf.ReloadInterval)
	} else {
		writePidFile("")
		log.Info("golia watch process start")
		os.Exit(restartWithReloader())
	}
}
