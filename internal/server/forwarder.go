package server

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Forwarder handles traffic forwarding through Wireguard to K8s services
type Forwarder struct {
	wireguardInterface string
	udpSessions        map[string]*udpSession
	udpMu              sync.RWMutex
	logger             *slog.Logger
}

// udpSession represents a pseudo-connection for UDP traffic
type udpSession struct {
	clientAddr *net.UDPAddr
	targetConn *net.UDPConn
	lastActive time.Time
	mu         sync.Mutex
}

// NewForwarder creates a new traffic forwarder
func NewForwarder(wireguardInterface string, logger *slog.Logger) *Forwarder {
	f := &Forwarder{
		wireguardInterface: wireguardInterface,
		udpSessions:        make(map[string]*udpSession),
		logger:             logger,
	}

	// Start UDP session cleanup goroutine
	go f.cleanupUDPSessions()

	return f
}

// ForwardTCP forwards TCP traffic to the target service
func (f *Forwarder) ForwardTCP(client net.Conn, targetIP string, targetPort int32) error {
	defer client.Close()

	// Dial target via Wireguard interface
	target, err := f.dialViaWireguard("tcp", fmt.Sprintf("%s:%d", targetIP, targetPort))
	if err != nil {
		return fmt.Errorf("failed to dial target: %w", err)
	}
	defer target.Close()

	f.logger.Debug("TCP connection established", "target", fmt.Sprintf("%s:%d", targetIP, targetPort))

	// Bidirectional copy
	errCh := make(chan error, 2)

	// Client -> Target
	go func() {
		_, err := io.Copy(target, client)
		errCh <- err
	}()

	// Target -> Client
	go func() {
		_, err := io.Copy(client, target)
		errCh <- err
	}()

	// Wait for first error or completion
	err = <-errCh

	// Note: We don't wait for the second goroutine to finish
	// Closing the connections will cause both to terminate

	if err != nil && err != io.EOF {
		return fmt.Errorf("forwarding error: %w", err)
	}

	f.logger.Debug("TCP connection closed", "target", fmt.Sprintf("%s:%d", targetIP, targetPort))
	return nil
}

// ForwardUDP forwards UDP packets to the target service
func (f *Forwarder) ForwardUDP(serverConn *net.UDPConn, clientAddr *net.UDPAddr, data []byte, targetIP string, targetPort int32) error {
	sessionKey := clientAddr.String()

	// Get or create session
	f.udpMu.Lock()
	session, exists := f.udpSessions[sessionKey]
	if !exists {
		// Create new session
		targetAddr := fmt.Sprintf("%s:%d", targetIP, targetPort)
		targetUDPAddr, err := net.ResolveUDPAddr("udp", targetAddr)
		if err != nil {
			f.udpMu.Unlock()
			return fmt.Errorf("failed to resolve target address: %w", err)
		}

		// Dial target
		targetConn, err := f.dialUDPViaWireguard(targetUDPAddr)
		if err != nil {
			f.udpMu.Unlock()
			return fmt.Errorf("failed to dial UDP target: %w", err)
		}

		session = &udpSession{
			clientAddr: clientAddr,
			targetConn: targetConn,
			lastActive: time.Now(),
		}
		f.udpSessions[sessionKey] = session

		f.logger.Debug("UDP session created", "client", clientAddr, "target", targetAddr)

		// Start goroutine to forward responses back to client
		go f.forwardUDPResponses(serverConn, session, sessionKey)
	}
	f.udpMu.Unlock()

	// Update last active time
	session.mu.Lock()
	session.lastActive = time.Now()
	session.mu.Unlock()

	// Forward packet to target
	if _, err := session.targetConn.Write(data); err != nil {
		return fmt.Errorf("failed to write to target: %w", err)
	}

	f.logger.Debug("UDP packet forwarded", "client", clientAddr, "size", len(data))
	return nil
}

// forwardUDPResponses forwards UDP responses from target back to client
func (f *Forwarder) forwardUDPResponses(serverConn *net.UDPConn, session *udpSession, sessionKey string) {
	buffer := make([]byte, 65535) // Max UDP packet size

	for {
		// Set read timeout
		session.targetConn.SetReadDeadline(time.Now().Add(30 * time.Second))

		n, err := session.targetConn.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Check if session is still active
				session.mu.Lock()
				inactive := time.Since(session.lastActive) > 5*time.Minute
				session.mu.Unlock()

				if inactive {
					f.logger.Debug("UDP session timed out", "client", session.clientAddr)
					f.removeUDPSession(sessionKey)
					return
				}
				continue
			}

			f.logger.Error("UDP read error", "error", err)
			f.removeUDPSession(sessionKey)
			return
		}

		// Update last active time
		session.mu.Lock()
		session.lastActive = time.Now()
		session.mu.Unlock()

		// Forward response to client
		if _, err := serverConn.WriteToUDP(buffer[:n], session.clientAddr); err != nil {
			f.logger.Error("Failed to write UDP response to client", "error", err)
			continue
		}

		f.logger.Debug("UDP response forwarded", "client", session.clientAddr, "size", n)
	}
}

// removeUDPSession removes a UDP session
func (f *Forwarder) removeUDPSession(sessionKey string) {
	f.udpMu.Lock()
	defer f.udpMu.Unlock()

	if session, exists := f.udpSessions[sessionKey]; exists {
		session.targetConn.Close()
		delete(f.udpSessions, sessionKey)
	}
}

// cleanupUDPSessions periodically cleans up inactive UDP sessions
func (f *Forwarder) cleanupUDPSessions() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		f.udpMu.Lock()
		now := time.Now()
		for key, session := range f.udpSessions {
			session.mu.Lock()
			inactive := now.Sub(session.lastActive) > 5*time.Minute
			session.mu.Unlock()

			if inactive {
				f.logger.Debug("Cleaning up inactive UDP session", "client", session.clientAddr)
				session.targetConn.Close()
				delete(f.udpSessions, key)
			}
		}
		f.udpMu.Unlock()
	}
}

// dialViaWireguard dials a TCP connection via the Wireguard interface
func (f *Forwarder) dialViaWireguard(network, address string) (net.Conn, error) {
	// For now, we'll use the default dialer
	// In production, you might want to bind to a specific interface
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	conn, err := dialer.Dial(network, address)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// dialUDPViaWireguard dials a UDP connection via the Wireguard interface
func (f *Forwarder) dialUDPViaWireguard(targetAddr *net.UDPAddr) (*net.UDPConn, error) {
	// For now, we'll use the default dialer
	// In production, you might want to bind to a specific interface
	conn, err := net.DialUDP("udp", nil, targetAddr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// Close closes the forwarder and all active sessions
func (f *Forwarder) Close() {
	f.udpMu.Lock()
	defer f.udpMu.Unlock()

	for key, session := range f.udpSessions {
		session.targetConn.Close()
		delete(f.udpSessions, key)
	}

	f.logger.Info("Forwarder closed")
}
