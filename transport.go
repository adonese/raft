package raft

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"time"
)

type SecureTransport struct {
	config    *tls.Config
	localAddr net.Addr
	listener  net.Listener
}

func NewSecureTransport(certFile, keyFile, caFile string) (*SecureTransport, error) {
	// Load certificates and set up TLS configuration
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}

	return &SecureTransport{config: tlsConfig}, nil
}

func (st *SecureTransport) Dial(addr string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	st.localAddr = conn.LocalAddr()
	return tls.Client(conn, st.config), nil
}

func (st *SecureTransport) Accept(listener net.Listener) (net.Conn, error) {
	tlsListener := tls.NewListener(listener, st.config)
	conn, err := tlsListener.Accept()
	if err != nil {
		return nil, err
	}
	st.localAddr = conn.LocalAddr()
	return conn, nil
}

func (st *SecureTransport) LocalAddr() net.Addr {
	return st.localAddr
}
