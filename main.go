package main

/* vim: set ts=2 sw=2 sts=2 ff=unix ft=go noet: */

import (
	"bufio"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var log = logrus.New()
var reason string

func init() {
	//log.SetLevel(logrus.DebugLevel)
}

func makeTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func dateString() string {
	return time.Now().Format("2 15:04:05")
}

func AlarmConnection() {
	log.Warnf("AlarmConnection reason: %s", reason)
	say := "lost.mp3"
	if reason == "EOF" {
		say = "server_lost.mp3"
	} else if matched, _ := regexp.MatchString(`^read tcp .+: i/o timeout$`, reason); matched {
		say = "connection_lost.mp3"
	}

	playSound(say)

	os.Exit(2)
}

func playSound(name string) {
	cmd := exec.Command("mplayer", name)
	log.Infof("mplayer %s", name)
	err := cmd.Run()
	if err != nil {
		log.Errorf("Error in run player: $v", err)
	}
}

func waitConnection(tcpAddr *net.TCPAddr) (conn *net.TCPConn) {
	var delay = 200
	var factor = 2.0
	var maxsleep = 5000
	var err error
	for {
		conn, err = net.DialTCP("tcp", nil, tcpAddr)
		if err != nil {
			log.Warnf("Dial failed: %s", err.Error())
			time.Sleep(time.Duration(delay) * time.Millisecond)
			delay = int(math.Ceil(float64(delay) * factor))
			if delay > maxsleep {
				delay = maxsleep
			}
		} else {
			playSound("connected.mp3")
			break
		}
	}

	return conn
}

func monitor(tcpAddr *net.TCPAddr) {
	timeoutDuration := 2500 * time.Millisecond
	slowDetectionMS := 138
	lagDetectionMS := 400

	conn := waitConnection(tcpAddr)

	defer func() {
		log.Info("Disconnect")
		conn.Close()
		AlarmConnection()
	}()

	bufReader := bufio.NewReader(conn)
	bufWriter := bufio.NewWriter(conn)
	var ts int64
	for {
		ts = makeTimestamp()
		_, err := fmt.Fprintf(bufWriter, "%d\n", ts)
		if err != nil {
			log.Info("Write to server failed:", err.Error())
			reason = err.Error()
			break
		}

		bufWriter.Flush()
		log.Debugf(">%d", ts)
		conn.SetReadDeadline(time.Now().Add(timeoutDuration))
		netData, err := bufReader.ReadString('\n')
		if err != nil {
			log.Errorf("Read ERROR: [%s]", err.Error())
			reason = err.Error()
			break
		}

		cmd := strings.TrimSpace(netData)
		log.Debugf("<%s", cmd)
		tsSend, err := strconv.ParseInt(cmd, 10, 64)
		if err != nil {
			log.Errorf("Cant convert to int %s: %s", cmd, err)
			continue
		}

		diffMs := int(makeTimestamp() - tsSend)
		log.Debugf("Ping: %d", diffMs)
		if diffMs > lagDetectionMS {
			log.Warnf("[%s] Lag detected: %dms", dateString(), diffMs)
		} else if diffMs > slowDetectionMS {
			log.Infof("[%s] Slow detected: %dms", dateString(), diffMs)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func main() {
	servAddr := os.Getenv("TCPING_ADDR")
	if len(servAddr) == 0 {
		log.Fatal("Required env parameter TCPING_ADDR [host:port] is not set")
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", servAddr)
	if err != nil {
		log.Info("ResolveTCPAddr failed:", err.Error())
		os.Exit(1)
	}

	for {
		monitor(tcpAddr)
	}
}
