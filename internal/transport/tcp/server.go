package tcp

import (
	"bufio"
	"context"
	"net"
	"strings"

	"go-im/internal/auth"
	"go-im/internal/cache"
)

type Server struct {
	Addr      string
	JWTSecret string
}

func (s *Server) Start(ctx context.Context) error {
	if s.Addr == "" {
		return nil
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	go func() { <-ctx.Done(); ln.Close() }()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, c net.Conn) {
	defer c.Close()
	reader := bufio.NewReader(c)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	cl, err := auth.ParseJWT(s.JWTSecret, line)
	if err != nil {
		return
	}
	sub := cache.Client().Subscribe(ctx, cache.DeliverChannel(cl.UserID))
	for {
		msg, err := sub.ReceiveMessage(ctx)
		if err != nil {
			return
		}
		c.Write([]byte(msg.Payload))
		c.Write([]byte("\n"))
	}
}
