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
	ReloadInterval int
	MetricInterval int
	CarbonAddr     string
	MondoAddr      string
	Metrics        []string
	LogLevel       string
}

func Init() {
	var format = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`)
	var logBackend = logging.NewLogBackend(os.Stderr, "", 0)
	logging.SetFormatter(format)
	logging.SetBackend(logBackend)

}

var log = logging.MustGetLogger("golia")
var ch = make(chan Datapoint)
var conn *Conn
var conf Config

func collectAndSend(addr string) {
	conn, err := NewConn(addr)
	if err == nil && conn != nil {
		for {
			dp := <-ch
			n, err1 := conn.WriteDataPoint(dp)
			if err1 != nil || n == 0 {
				log.Warningf("can't send metric %v", dp)
			}
		}
	} else {
		log.Error(err)
		os.Exit(2)
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

func reloaderLoop(path string, interval int) {
	var md5 string
	for {
		tag, err := getMd5(path)
		if err != nil {
			log.Error(err)
			os.Exit(4)
		}
		if md5 == "" {
			md5 = tag
		} else if tag != md5 {
			log.Info("detect change....")
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
		files[syscall.Stdin] = os.Stdin
		files[syscall.Stdout] = os.Stdout
		files[syscall.Stderr] = os.Stderr
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

func getAddrByDefault() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}
	defer conn.Close()
	return strings.Split(conn.LocalAddr().String(), ":")[0], nil
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
	level, ok := levels[config.Log_level]
	if !ok {
		log.Error("unrecognized log level '%s'\n", config.Log_level)
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
		metricHead := strings.Replace(ip, ".", "_", -1)
		collector := Collector{ch, metricHead, conf.ReloadInterval}
		go handleExit(sigs)
		go collectAndSend(conf.CarbonAddr)
		go collector.CollectAllMetric(conf.Metrics)
		reloaderLoop(config_file, conf.ReloadInterval)
	} else {
		log.Info("golia watch process start")
		os.Exit(restartWithReloader())
	}
}
