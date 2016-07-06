package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
)

func Test_datapoint(t *testing.T) {
	dp := Datapoint{"name.name.name", 6.12, 1467354594}
	if dps := "name.name.name 6.120000 1467354594\n"; dps != dp.formated() {
		t.Error("datapoint convert error")
	}
}

func Test_connect(t *testing.T) {
	cc := make(chan string)
	//go runDummyServer(cc, t)
	go func() { //start dummpy server
		ln, _ := net.Listen("tcp", ":2024")
		conn, _ := ln.Accept()
		//defer conn.Close()
		for {
			msg := <-cc
			message, _ := bufio.NewReader(conn).ReadString('\n')
			if strings.Trim(msg, "\n") != strings.Trim(message, "\n") {
				t.Error("message is not match")
			}
		}
	}()
	c, err := NewConn("127.0.0.1:2024")
	if err == nil {
		msg := "helloworld"
		cc <- msg
		c.Write([]byte(msg))
		if c.isAlive() == false {
			t.Error("agent should be alive")
		}
		msg = "helloworld" + "2"
		cc <- msg
		c.Write([]byte(msg))
		msg = "helloworld" + "3"
		cc <- msg
		c.Write([]byte(msg))
		c.Close()
	} else {
		fmt.Println(err)
	}
}
