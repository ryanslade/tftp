package main

import (
	"bytes"
	"io"
	"net"
	"time"
)

type mockHandler struct {
	replyChan chan struct{}
}

func (m *mockHandler) serve(remoteAddr net.Addr, filename string) {
	m.replyChan <- struct{}{}
}

type mockAddr struct{}

func (m mockAddr) Network() string {
	return "udp"
}

func (m mockAddr) String() string {
	return "mockAddr"
}

type mockPacketConn struct {
	data *bytes.Buffer
	addr net.Addr
}

func (m *mockPacketConn) ReadFrom(b []byte) (n int, add net.Addr, err error) {
	to := bytes.NewBuffer(b)
	to.Truncate(0)
	n64, err := io.Copy(to, m.data)
	return int(n64), m.addr, err
}

func (m *mockPacketConn) WriteTo(b []byte, add net.Addr) (n int, err error) {
	from := bytes.NewReader(b)
	n64, err := io.Copy(m.data, from)
	return int(n64), err
}

func (m *mockPacketConn) Close() error {
	return nil
}

func (m *mockPacketConn) LocalAddr() net.Addr {
	return m.addr
}

func (m *mockPacketConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockPacketConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockPacketConn) SetWriteDeadline(t time.Time) error {
	return nil
}
