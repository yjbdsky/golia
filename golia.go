package main

import (
	"os"

	logging "github.com/op/go-logging"
)

func Init() {
	var format = "%{color}%{time:15:04:05.000000} %{level:.4s} %{color:reset} %{message}"
	logBackend := logging.NewLogBackend(os.Stderr, "", 0)
	logging.SetFormatter(logging.MustStringFormatter(format))
	logging.SetBackend(logBackend)
}

var log = logging.MustGetLogger("gometric")
