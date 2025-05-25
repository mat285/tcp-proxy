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
	upstreams      map[int][]upstream
	upstreamsIndex map[int]int
	upstreamLock   *sync.Mutex
)

func main() {
	fromPortEnv := "PROXY_PORTS"
	fromPortEnvValue, has := os.LookupEnv(fromPortEnv)
	if !has {
		log.Fatalf("Environment variable %s is not set", fromPortEnv)
		os.Exit(1)
	}
	fromPorts := strings.Split(fromPortEnvValue, ",")
	if len(fromPorts) == 0 {
		log.Fatalf("Environment variable %s is not set", fromPortEnv)
		os.Exit(1)
	}

	upstreams = make(map[int][]upstream)
	upstreamsIndex = make(map[int]int)
	upstreamLock = &sync.Mutex{}

	parsedPorts := make([]int, 0, len(fromPorts))
	for _, port := range fromPorts {
		port = strings.TrimSpace(port)
		if port == "" {
			continue
		}
		fromPort, err := strconv.Atoi(port)
		if err != nil || fromPort <= 0 {
			log.Fatalf("Invalid port number in %s: %s", fromPortEnv, port)
			os.Exit(1)
		}
		parsedPorts = append(parsedPorts, fromPort)
		upstreamEnvs := fmt.Sprintf("PROXY_UPSTREAMS_%d", fromPort)
		upstreamEnvValue, has := os.LookupEnv(upstreamEnvs)
		if !has {
			log.Fatalf("Environment variable %s is not set", upstreamEnvs)
			os.Exit(1)
		}
		upstreamStrs := strings.Split(upstreamEnvValue, ",")
		if len(upstreamStrs) == 0 {
			log.Fatalf("Environment variable %s is not set", upstreamEnvs)
			os.Exit(1)
		}
		if err := parseUpstreams(upstreamStrs, fromPort); err != nil {
			log.Fatalf("Error parsing upstreams: %v", err)
			os.Exit(1)
		}
	}

	var wg sync.WaitGroup
	for _, fromPort := range parsedPorts {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			if err := listenPort(port); err != nil {
				log.Printf("Error listening on port %d: %v", port, err)
				os.Exit(1)
			}
		}(fromPort)
	}
	wg.Wait()
	log.Println("All listeners have exited")
}

func listenPort(port int) error {
	log.Printf("Starting proxy from port %d", port)

	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: port,
		IP:   net.ParseIP("0.0.0.0"),
	})

	if err != nil {
		return err
	}
	defer listener.Close()
	log.Printf("Listening on port %d", port)

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		go handleConnection(conn, port)
	}
}

func handleConnection(conn *net.TCPConn, port int) {
	defer conn.Close()
	log.Printf("Accepted connection from %s", conn.RemoteAddr().String())

	// Connect to the target port
	upstreamConn, err := getNextUpstream(port)
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

func parseUpstreams(upstreamStrs []string, port int) error {
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

	upstreamsIndex[port] = 0
	upstreams[port] = parsed
	return nil
}

func getNextUpstream(port int) (net.Conn, error) {
	for i := 0; i < len(upstreams); i++ {
		upstreamLock.Lock()
		upstreamsIndex[port] = (upstreamsIndex[port] + 1) % len(upstreams[port])
		upstreamLock.Unlock()
		upstream := upstreams[port][upstreamsIndex[port]]
		upstreamConn, err := net.Dial("tcp", upstream.IP+":"+strconv.Itoa(upstream.Port))
		if err != nil {
			log.Printf("Error connecting to upstream %s:%d: %v", upstream.IP, upstream.Port, err)
			continue
		}
		return upstreamConn, nil
	}
	return nil, fmt.Errorf("no available upstreams")
}
