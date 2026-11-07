//go:build ignore

// The same very small HTTP/1.1 server as server.brick, in Go.
//
// Same shape as the Brick and C versions: one goroutine, one accept loop, one
// blocking read per connection, one scan for the CR, one write, close. No
// net/http (that would be a different program), just net.Listen and net.Conn.
//
// Build:
//	go build -o bench/server/build/go-server bench/server/server-go.go
package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

const okResponse = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 14\r\nConnection: close\r\n\r\nHello, Brick!\n"

const badResponse = "HTTP/1.1 404 Not Found\r\nContent-Type: text/plain\r\nContent-Length: 4\r\nConnection: close\r\n\r\nnope"

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		if _, err := strconv.Atoi(p); err == nil {
			return p
		}
	}
	return "18080"
}

// handle reads one request, answers it and closes: one request per connection,
// exactly like the other three servers.
func handle(conn net.Conn) {
	buf := make([]byte, 8192)
	n, _ := conn.Read(buf)
	line := n
	for i := 0; i < n; i++ {
		if buf[i] == '\r' || buf[i] == 0 {
			line = i
			break
		}
	}
	if line >= 4 && string(buf[:4]) == "GET " {
		conn.Write([]byte(okResponse))
	} else {
		conn.Write([]byte(badResponse))
	}
	conn.Close()
}

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:"+port())
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-server: cannot listen")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "go-server: listening")
	for {
		conn, err := listener.Accept()
		if err != nil {
			os.Exit(1)
		}
		handle(conn)
	}
}
