package server

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/noahjeana/k8s-exposer/pkg/types"
)

// PortListener manages a listener for a specific port and protocol
type PortListener struct {
	port      int32
	protocol  string
	target    types.ExposedService
	forwarder *Forwarder
	logger    *slog.Logger

	// For TCP
	tcpListener net.Listener

	// For UDP
	udpConn *net.UDPConn

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewPortListener creates a new port listener
func NewPortListener(port int32, protocol string, target types.ExposedService, forwarder *Forwarder, logger *slog.Logger) *PortListener {
	return &PortListener{
		port:      port,
		protocol:  protocol,
		target:    target,
		forwarder: forwarder,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// Start starts the port listener
func (pl *PortListener) Start() error {
	pl.logger.Info("Starting listener",
		"subdomain", pl.target.Subdomain,
		"port", pl.port,
		"protocol", pl.protocol,
		"target", fmt.Sprintf("%s:%d", pl.target.TargetIP, pl.getTargetPort()))

	switch pl.protocol {
	case "tcp":
		return pl.startTCP()
	case "udp":
		return pl.startUDP()
	case "tcp+udp":
		if err := pl.startTCP(); err != nil {
			return err
		}
		if err := pl.startUDP(); err != nil {
			pl.stopTCP()
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported protocol: %s", pl.protocol)
	}
}

// startTCP starts a TCP listener
func (pl *PortListener) startTCP() error {
	// Bind explicitly to 0.0.0.0 (IPv4) to ensure HAProxy can connect via localhost/127.0.0.1
	listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", pl.port))
	if err != nil {
		return fmt.Errorf("failed to start TCP listener: %w", err)
	}

	pl.tcpListener = listener

	pl.wg.Add(1)
	go pl.acceptTCPConnections()

	pl.logger.Info("TCP listener started", "port", pl.port)
	return nil
}

// startUDP starts a UDP listener
func (pl *PortListener) startUDP() error {
	addr := &net.UDPAddr{
		Port: int(pl.port),
		IP:   net.IPv4zero,
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to start UDP listener: %w", err)
	}

	pl.udpConn = conn

	pl.wg.Add(1)
	go pl.receiveUDPPackets()

	pl.logger.Info("UDP listener started", "port", pl.port)
	return nil
}

// acceptTCPConnections accepts incoming TCP connections
func (pl *PortListener) acceptTCPConnections() {
	defer pl.wg.Done()

	for {
		conn, err := pl.tcpListener.Accept()
		if err != nil {
			select {
			case <-pl.stopCh:
				return
			default:
				pl.logger.Error("Failed to accept TCP connection", "error", err)
				continue
			}
		}

		pl.logger.Debug("TCP connection accepted", "remote", conn.RemoteAddr())

		// Handle connection in a new goroutine
		go pl.handleTCPConnection(conn)
	}
}

// handleTCPConnection handles a single TCP connection
func (pl *PortListener) handleTCPConnection(conn net.Conn) {
	targetPort := pl.getTargetPort()

	pl.logger.Debug("Forwarding TCP connection",
		"client", conn.RemoteAddr(),
		"target", fmt.Sprintf("%s:%d", pl.target.TargetIP, targetPort))

	if err := pl.forwarder.ForwardTCP(conn, pl.target.TargetIP, targetPort); err != nil {
		pl.logger.Error("TCP forwarding failed", "error", err)
	}
}

// receiveUDPPackets receives and forwards UDP packets
func (pl *PortListener) receiveUDPPackets() {
	defer pl.wg.Done()

	buffer := make([]byte, 65535) // Max UDP packet size

	for {
		select {
		case <-pl.stopCh:
			return
		default:
		}

		n, clientAddr, err := pl.udpConn.ReadFromUDP(buffer)
		if err != nil {
			select {
			case <-pl.stopCh:
				return
			default:
				pl.logger.Error("Failed to read UDP packet", "error", err)
				continue
			}
		}

		pl.logger.Debug("UDP packet received", "client", clientAddr, "size", n)

		// Forward packet
		targetPort := pl.getTargetPort()
		data := make([]byte, n)
		copy(data, buffer[:n])

		go func() {
			if err := pl.forwarder.ForwardUDP(pl.udpConn, clientAddr, data, pl.target.TargetIP, targetPort); err != nil {
				pl.logger.Error("UDP forwarding failed", "error", err)
			}
		}()
	}
}

// Stop stops the port listener
func (pl *PortListener) Stop() error {
	pl.logger.Info("Stopping listener", "port", pl.port, "protocol", pl.protocol)

	close(pl.stopCh)

	if pl.tcpListener != nil {
		pl.stopTCP()
	}

	if pl.udpConn != nil {
		pl.stopUDP()
	}

	pl.wg.Wait()

	pl.logger.Info("Listener stopped", "port", pl.port, "protocol", pl.protocol)
	return nil
}

// stopTCP stops the TCP listener
func (pl *PortListener) stopTCP() {
	if pl.tcpListener != nil {
		pl.tcpListener.Close()
		pl.tcpListener = nil
	}
}

// stopUDP stops the UDP listener
func (pl *PortListener) stopUDP() {
	if pl.udpConn != nil {
		pl.udpConn.Close()
		pl.udpConn = nil
	}
}

// getTargetPort returns the target port for this listener
func (pl *PortListener) getTargetPort() int32 {
	// Find the matching port in the target service
	for _, portMapping := range pl.target.Ports {
		if portMapping.Protocol == pl.protocol || portMapping.Protocol == "tcp+udp" {
			// Use TargetPort if available (for NodePort services), otherwise use Port
			if portMapping.TargetPort != 0 {
				return portMapping.TargetPort
			}
			return portMapping.Port
		}
	}
	// Fallback to the listener port
	return pl.port
}
