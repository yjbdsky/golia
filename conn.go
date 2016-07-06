package main

import (
	"fmt"
	"io"
	"net"
)

var conn_in_buffer = 300
var newLine = []byte{'\n'}

type Conn struct {
	conn     *net.TCPConn // which is also io.Writer
	addr     string
	shutdown chan bool
	In       chan []byte
	up       bool
	updateUp chan bool
	checkUp  chan bool
	pickle   bool
}

type Datapoint struct {
	Name string
	Val  float64
	Time uint32
}

func NewConn(addr string) (*Conn, error) {
	raddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	laddr, _ := net.ResolveTCPAddr("tcp", "0.0.0.0")
	conn, err := net.DialTCP("tcp", laddr, raddr)
	if err != nil {
		return nil, err
	}
	connObj := &Conn{
		conn:     conn,
		shutdown: make(chan bool, 1),
		In:       make(chan []byte, conn_in_buffer),
		up:       true,
		updateUp: make(chan bool),
		checkUp:  make(chan bool),
		pickle:   false,
		addr:     addr,
	}
	go connObj.checkEOF()
	go connObj.handleData()
	go connObj.handleStatus()
	return connObj, err
}

func (c *Conn) isAlive() bool {
	return <-c.checkUp
}

func (c *Conn) handleStatus() {
	for {
		select {
		case c.up = <-c.updateUp:
			log.Infof("conn %s up set to %t", c.addr, c.up)
		case c.checkUp <- c.up:
			log.Infof("conn %s up query responded with %t", c.addr, c.up)
		}
	}
}

func (d *Datapoint) formated() string {
	return fmt.Sprintf("%s %f %d\n", d.Name, d.Val, d.Time)
}

func (c *Conn) WriteDataPoint(data Datapoint) (int, error) {
	s := data.formated()
	return c.Write([]byte(s))
}

func (c *Conn) Write(buf []byte) (int, error) {
	written := 0
	size := len(buf)
	n, err := c.conn.Write(buf)
	written += n
	if err == nil && size == n && !c.pickle {
		size = 1
		n, err = c.conn.Write(newLine)
		written += n
	}
	if err != nil {
		log.Infof("write success, but size not match,")
	}
	if err == nil && size != n {
		//c.numErrTruncated.Inc(1)
		err = fmt.Errorf("truncated write: %s", buf)
	}
	return written, err
}

func (c *Conn) checkEOF() {
	b := make([]byte, 1024)
	for {
		num, err := c.conn.Read(b)
		if err == io.EOF {
			log.Infof("conn %s .conn.Read returned EOF -> conn is closed. closing conn explicitly", c.addr)
			c.Close()
			return
		}
		// just in case i misunderstand something or the remote behaves badly
		if num != 0 {
			log.Infof("conn %s .conn.Read data? did not expect that.  data: %s", c.addr, b[:num])
		}
		if err != io.EOF {
			log.Infof("conn %s checkEOF .conn.Read returned err != EOF, which is unexpected.  closing conn. error: %s", c.addr, err)
			c.Close()
			return
		}
	}
}

func (c *Conn) Close() error {
	c.updateUp <- false // redundant in case handleData() called us, but not if the dest called us
	log.Infof("conn %s Close() called. sending shutdown", c.addr)
	c.shutdown <- true
	log.Infof("conn %s c.conn.Close()", c.addr)
	a := c.conn.Close()
	log.Infof("conn %s c.conn is closed", c.addr)
	return a
}

func (c *Conn) handleData() {
	for {
		select { // handle incoming data or flush/shutdown commands
		case buf := <-c.In:
			_, err := c.Write(buf)
			if err != nil {
				log.Infof("conn %s write error: %s", c.addr, err)
				log.Infof("conn %s setting up=false", c.addr)
				c.updateUp <- false // assure In won't receive more data because every loop that writes to In reads this out
				log.Infof("conn %s Closing", c.addr)
				go c.Close() // this can take a while but that's ok. this conn won't be used anymore
				return
			}
		case <-c.shutdown:
			log.Infof("conn %s handleData: shutdown received. returning.", c.addr)
			return
		}
		log.Infof("conn %s handleData", c.addr)
	}
}
