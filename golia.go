package main

import (
	"crypto/md5"
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

	logging "github.com/op/go-logging"
)

const (
	SIGINT  = syscall.SIGINT
	SIGQUIT = syscall.SIGQUIT
	SIGTERM = syscall.SIGTERM
)

func Init() {
	var format = "%{color}%{time:15:04:05.000000} %{level:.4s} %{color:reset} %{message}"
	logBackend := logging.NewLogBackend(os.Stderr, "", 0)
	logging.SetFormatter(logging.MustStringFormatter(format))
	logging.SetBackend(logBackend)
}



var log = logging.MustGetLogger("golia")
ch := make(chan Datapoint)
var conn *Conn

sleepInterval:=30
fileToWatch:="a.txt"
carbonAddr:="op4.ecld.com:2003"
mondoAddr:="http://op2.ecld.com:9518/"
metricHead:=strings.Replace(GetAddr(), ".", "_", -1)


func collectAndSend(addr string) {
	c, err := NewConn(addr)
	if err != nil {
		conn = &c
		for{
			dp := <-ch
			conn.WriteDataPoint(dp)
		}
	} else {
		log.Error(err)
		return
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
		log.Infof("value is %s\n", tag)
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
		log.Debug("no change...")
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
	conn.Close()
	log.Infof("exited...%v\n", sig)
	os.Exit(0)
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

func getAddrByDefault() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		fmt.Println(err.Error())
		return "Erorr"
	}
	defer conn.Close()
	return strings.Split(conn.LocalAddr().String(), ":")[0]
}

func GetAddr() (r string) {
	resp, err := http.Get(mondoAddr)
	defer resp.Body.Close()
	if err != nil {
		r = getAddrByDefault()
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil || r == "" {
		r = getAddrByDefault()
	}
	r = string(body)
	return
}

func main() {
	if os.Getenv("RUN_MAIN") == "true" {
		log.Info("golia subprocess start")
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go handleExit(sigs)
		collectAndSend(carbonAddr)
		collector :=Collector{ch,metricHead,sleepInterval}
		go collector.CollectAllMetric()
		reloaderLoop(fileToWatch, sleepInterval)
	} else {
		log.Info("golia watch process start")
	}
	os.Exit(restartWithReloader())
}
