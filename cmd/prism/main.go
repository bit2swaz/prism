package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/bit2swaz/prism/internal/engine"
	"github.com/bit2swaz/prism/internal/protocol"
	"github.com/bit2swaz/prism/internal/storage"
)

func main() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	port := "5432"

	driver := storage.NewBtrfsDriver("/mnt/prism_data")
	if err := driver.Init(); err != nil {
		logger.Error("Storage Init failed", "error", err)
		os.Exit(1)
	}

	dockerMgr, err := engine.NewManager()
	if err != nil {
		logger.Error("Docker Engine Init failed", "error", err)
		os.Exit(1)
	}

	dockerMgr.StartReaper(10*time.Second, 30*time.Second, logger)
	logger.Info("Reaper Activated", "idle_timeout", "30s")

	go func() {
		http.HandleFunc("/branches", func(w http.ResponseWriter, r *http.Request) {
			branches := dockerMgr.ListBranches()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(branches)
		})

		logger.Info("Management API Live", "port", "8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			logger.Error("API Server failed", "error", err)
		}
	}()

	listener, err := net.Listen("tcp", "0.0.0.0:"+port)
	if err != nil {
		logger.Error("Failed to bind port", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	logger.Info("Prism Gateway Live", "port", port, "storage", "Btrfs")

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("Accept error", "error", err)
			continue
		}

		go handleConnection(conn, logger, driver, dockerMgr)
	}
}

func Pipe(client net.Conn, backend net.Conn) {
	done := make(chan struct{})

	go func() {
		io.Copy(backend, client)
		client.Close()
		done <- struct{}{}
	}()

	go func() {
		io.Copy(client, backend)
		backend.Close()
		done <- struct{}{}
	}()

	<-done
}

func handleConnection(clientConn net.Conn, logger *slog.Logger, driver *storage.BtrfsDriver, mgr *engine.Manager) {
	defer clientConn.Close()

	remoteAddr := clientConn.RemoteAddr().String()

	msg, err := protocol.ParseStartup(clientConn)
	if err != nil {
		logger.Error("Handshake failed", "remote", remoteAddr, "error", err)
		return
	}

	realUser, branchID := protocol.ExtractBranch(msg.User)
	logger.Info("Request Received", "branch", branchID, "user", realUser)

	_, err = driver.CreateSnapshot("master")
	if err != nil {
		logger.Warn("Snapshot creation note", "error", err)
	}

	mountPath, err := driver.Clone("snap_master", branchID)
	if err != nil {
		logger.Error("Branching failed", "branch", branchID, "error", err)
		return
	}

	containerIP, err := mgr.SpinUp(branchID, mountPath)
	if err != nil {
		logger.Error("Container start failed", "branch", branchID, "error", err)
		return
	}

	logger.Info("Infrastructure Ready", "branch", branchID, "ip", containerIP)

	var backendConn net.Conn
	var dialErr error
	maxRetries := 30
	retryInterval := 500 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		logger.Debug("Dialing backend", "attempt", i+1, "ip", containerIP)
		backendConn, dialErr = net.DialTimeout("tcp", containerIP, 2*time.Second)
		if dialErr == nil {
			break
		}
		logger.Debug("Backend unreachable, retrying...", "error", dialErr)
		time.Sleep(retryInterval)
	}

	if dialErr != nil {
		logger.Error("Backend connection failed after retries", "ip", containerIP, "error", dialErr)
		return
	}
	defer backendConn.Close()

	logger.Info("Backend Connected", "ip", containerIP)

	if err := sendStartupPacket(backendConn, realUser, msg.Database); err != nil {
		logger.Error("Failed to forward startup", "error", err)
		return
	}

	logger.Info("Startup Packet Sent")
	logger.Info("Proxying Traffic", "client", remoteAddr, "backend", containerIP)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mgr.Touch(branchID)
			case <-ctx.Done():
				return
			}
		}
	}()

	Pipe(clientConn, backendConn)
}

func sendStartupPacket(conn net.Conn, user, db string) error {
	var buf []byte

	buf = append(buf, []byte("user\x00")...)
	buf = append(buf, []byte(user+"\x00")...)
	buf = append(buf, []byte("database\x00")...)
	buf = append(buf, []byte(db+"\x00")...)
	buf = append(buf, 0x00)

	totalLen := len(buf) + 8

	header := make([]byte, 8)
	header[0] = byte(totalLen >> 24)
	header[1] = byte(totalLen >> 16)
	header[2] = byte(totalLen >> 8)
	header[3] = byte(totalLen)
	header[4] = 0x00
	header[5] = 0x03
	header[6] = 0x00
	header[7] = 0x00

	if _, err := conn.Write(header); err != nil {
		return err
	}

	if _, err := conn.Write(buf); err != nil {
		return err
	}
	return nil
}
