package main

import (
	"flag"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/jsiebens/coproxy/dns"
	"github.com/jsiebens/coproxy/strategy"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

var (
	port         = flag.Int("port", 7000, "Port to bind the server to")
	server       = flag.String("server", "", "The DNS servers to use")
	target       = flag.String("target", "", "The target service")
	loggerFormat = flag.String("logger_format", "text", "Format for log output text | json")
	loggerLevel  = flag.String("logger_level", "INFO", "Log output level INFO | ERROR | DEBUG | TRACE")
	loggerOutput = flag.String("logger_output", "", "Filepath to write log file, if omitted stdOut is used")
)

func main() {
	flag.Parse()

	logger := hclog.New(&hclog.LoggerOptions{
		Name:       "coproxy",
		Level:      hclog.LevelFromString(*loggerLevel),
		JSONFormat: strings.ToLower(*loggerFormat) == "json",
		Output:     createLogFile(),
	})

	var lookup dns.Lookup

	if len(*server) == 0 {
		lookup = dns.NewLookupLib(*server)
	} else {
		defaultLookup, err := dns.NewDefaultLookupLib()
		if err != nil {
			logger.Error("Error creating default DNS lookup strategy", "error", err)
			os.Exit(1)
		}
		lookup = defaultLookup
	}

	targets, err := lookup.Lookup(*target)
	if err != nil {
		logger.Error("Error reading targets", "error", err)
		os.Exit(1)
	}

	roundrobin := strategy.NewRoundRobin(logger, targets)

	go func(f string, rr strategy.Strategy, l dns.Lookup) {
		for {
			time.Sleep(5 * time.Second)
			t, err := l.Lookup(f)
			if err == nil {
				rr.Set(t)
			} else {
				logger.Error("Error reading targets", err)
			}
		}
	}(*target, roundrobin, lookup)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		logger.Error("Error starting listener", "error", err)
		os.Exit(1)
	}
	for {
		conn, err := listener.Accept()
		logger.Trace("New connection", "addr", conn.RemoteAddr())
		if err != nil {
			logger.Error("error accepting connection", "error", err)
			continue
		}
		go func(rr strategy.Strategy) {
			defer conn.Close()
			conn2, err := net.Dial("tcp", rr.Next())
			if err != nil {
				logger.Error("error dialing remote addr", "error", err)
				return
			}
			defer conn2.Close()
			closer := make(chan struct{}, 2)
			go pipe(closer, conn2, conn)
			go pipe(closer, conn, conn2)
			<-closer
			logger.Trace("Connection complete", "addr", conn.RemoteAddr())
		}(roundrobin)
	}
}

func pipe(closer chan struct{}, dst io.Writer, src io.Reader) {
	_, _ = io.Copy(dst, src)
	closer <- struct{}{} // connection is closed, send signal to stop proxy
}

func createLogFile() *os.File {
	if *loggerOutput != "" {
		f, err := os.OpenFile(*loggerOutput, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			return f
		}
		log.Printf("Unable to open file for output, defaulting to std out: %s\n", err.Error())
	}
	return os.Stdout
}
