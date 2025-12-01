package main

import (
	"log/slog"
	"net"
	"os"

	"github.com/bit2swaz/prism/internal/protocol"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	port := "5432"

	// 2. Start Listener
	listener, err := net.Listen("tcp", "0.0.0.0:"+port)
	if err != nil {
		logger.Error("Failed to bind port", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	logger.Info("Prism Gateway Started", "port", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("Accept error", "error", err)
			continue
		}

		go handleConnection(conn, logger)
	}
}

func handleConnection(conn net.Conn, logger *slog.Logger) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()

	msg, err := protocol.ParseStartup(conn)
	if err != nil {
		logger.Error("Protocol handshake failed", "remote", remoteAddr, "error", err)
		return
	}

	realUser, branchID := protocol.ExtractBranch(msg.User)

	logger.Info("Connection Intercepted",
		"remote", remoteAddr,
		"raw_user", msg.User,
		"target_user", realUser,
		"target_branch", branchID,
		"database", msg.Database,
	)

	// TODO: Spin up Docker container and proxy traffic
	logger.Warn("Backend not implemented yet. Closing connection.")
}
