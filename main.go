package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

type upstream struct {
	IP   string
	Port int
}

var (
	upstreams     []upstream
	upstreamIndex int
	upstreamLock  *sync.Mutex
)

func main() {
	fromPortEnv := "PROXY_PORT"
	upstreamEnvs := "PROXY_UPSTREAMS"
	fromPort, err := strconv.Atoi(os.Getenv(fromPortEnv))
	if err != nil || fromPort <= 0 {
		log.Fatalf("Environment variable %s is not set", fromPortEnv)
		os.Exit(1)
	}
	upstreamStrs := strings.Split(os.Getenv(upstreamEnvs), ",")
	if len(upstreamStrs) == 0 {
		log.Fatalf("Environment variable %s is not set", upstreamEnvs)
		os.Exit(1)
	}
	if err := parseUpstreams(upstreamStrs); err != nil {
		log.Fatalf("Error parsing upstreams: %v", err)
		os.Exit(1)
	}
	log.Printf("Starting proxy from port %d", fromPort)

	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: fromPort,
		IP:   net.ParseIP("0.0.0.0"),
	})

	if err != nil {
		log.Fatalf("Error starting TCP listener: %v", err)
		os.Exit(1)
	}
	defer listener.Close()
	log.Printf("Listening on port %d", fromPort)

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn *net.TCPConn) {
	defer conn.Close()
	log.Printf("Accepted connection from %s", conn.RemoteAddr().String())

	// Connect to the target port
	upstreamConn, err := getNextUpstream()
	if err != nil {
		log.Printf("Error connecting to upstream: %v", err)
		return
	}
	defer upstreamConn.Close()
	log.Printf("Connected to upstream %s", upstreamConn.RemoteAddr().String())
	// Start forwarding data between the two connections
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := io.Copy(conn, upstreamConn)
		if err != nil {
			log.Printf("Error forwarding data to client: %v", err)
			return
		}
		log.Printf("Done forwarding connection from %s to %s", upstreamConn.RemoteAddr().String(), conn.RemoteAddr().String())
	}()
	go func() {
		defer wg.Done()
		_, err = io.Copy(upstreamConn, conn)
		if err != nil {
			log.Printf("Error forwarding data to upstream: %v", err)
			return
		}
	}()
	wg.Wait()
	log.Printf("Done forwarding connection from %s to %s", conn.RemoteAddr().String(), upstreamConn.RemoteAddr().String())
}

func parseUpstreams(upstreamStrs []string) error {
	var parsed []upstream
	for _, upstreamStr := range upstreamStrs {
		parts := strings.Split(upstreamStr, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid upstream format: %s", upstreamStr)
		}
		ip := parts[0]
		port, err := strconv.Atoi(parts[1])
		if err != nil || port <= 0 {
			return fmt.Errorf("invalid port number: %s", parts[1])
		}
		parsed = append(parsed, upstream{IP: ip, Port: port})
	}
	upstreamLock = &sync.Mutex{}
	upstreamIndex = 0
	upstreams = parsed
	return nil
}

func getNextUpstream() (net.Conn, error) {
	for i := 0; i < len(upstreams); i++ {
		upstreamLock.Lock()
		upstreamIndex = (upstreamIndex + 1) % len(upstreams)
		upstreamLock.Unlock()
		upstream := upstreams[upstreamIndex]
		upstreamConn, err := net.Dial("tcp", upstream.IP+":"+strconv.Itoa(upstream.Port))
		if err != nil {
			log.Printf("Error connecting to upstream %s:%d: %v", upstream.IP, upstream.Port, err)
			continue
		}
		return upstreamConn, nil
	}
	return nil, fmt.Errorf("no available upstreams")
}
