package grpcutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// LoadServerTLS creates gRPC server credentials with mutual TLS.
// The CA cert is used to verify client certificates.
func LoadServerTLS(certFile, keyFile, caFile string) (grpc.ServerOption, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	return grpc.Creds(credentials.NewTLS(tlsCfg)), nil
}

// LoadClientTLS creates gRPC dial credentials with mutual TLS.
// The CA cert is used to verify the server's certificate.
func LoadClientTLS(certFile, keyFile, caFile string) (grpc.DialOption, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}

	return grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)), nil
}

// TLSFromEnv loads TLS configuration from standard environment variables.
// Returns nil options if SPARK_TLS_CERT is not set (graceful degradation for dev).
func TLSFromEnv() (serverOpt grpc.ServerOption, clientOpt grpc.DialOption, err error) {
	certFile := os.Getenv("SPARK_TLS_CERT")
	keyFile := os.Getenv("SPARK_TLS_KEY")
	caFile := os.Getenv("SPARK_TLS_CA")

	if certFile == "" {
		return nil, nil, nil // TLS not configured
	}

	serverOpt, err = LoadServerTLS(certFile, keyFile, caFile)
	if err != nil {
		return nil, nil, fmt.Errorf("server TLS: %w", err)
	}

	clientOpt, err = LoadClientTLS(certFile, keyFile, caFile)
	if err != nil {
		return nil, nil, fmt.Errorf("client TLS: %w", err)
	}

	return serverOpt, clientOpt, nil
}
