package server

import (
	"fmt"
	"google.golang.org/grpc"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"github.com/blk-io/crux/utils"
	"net"
	"net/http"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"google.golang.org/grpc/credentials"
)

func (tm *TransactionManager) startRpcServer(port int, ipcPath string, tls bool, certFile, keyFile string) error {
	lis, err := utils.CreateIpcSocket(ipcPath)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := Server{Enclave : tm.Enclave}
	grpcServer := grpc.NewServer()
	RegisterClientServer(grpcServer, &s)
	go func() {
		log.Fatal(grpcServer.Serve(lis))
	}()

	go func() error {
		var err error
		if tls {
			err = tm.startRestServerTLS(port, certFile, keyFile, certFile)
		} else {
			err = tm.startRestServer(port)
		}
		if err != nil {
			log.Fatalf("failed to start gRPC REST server: %s", err)
		}
		return err
	}()

	return err
}

func (tm *TransactionManager) startRestServer(port int) error {
	grpcAddress := fmt.Sprintf("%s:%d", "localhost", port-1)
	lis, err := net.Listen("tcp", grpcAddress)

	s := Server{Enclave : tm.Enclave}
	grpcServer := grpc.NewServer()
	RegisterClientServer(grpcServer, &s)
	go func() {
		log.Fatal(grpcServer.Serve(lis))
	}()

	address := fmt.Sprintf("%s:%d", "localhost", port)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err = RegisterClientHandlerFromEndpoint(ctx, mux, grpcAddress, opts)
	if err != nil {
		return fmt.Errorf("could not register service Ping: %s", err)
	}
	log.Printf("starting HTTP/1.1 REST server on %s", address)
	http.ListenAndServe(address, mux)
	return nil
}

func (tm *TransactionManager) startRestServerTLS(port int, certFile, keyFile, ca string) error {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("could not load server key pair: %s", err)
	}

	certPool := x509.NewCertPool()
	cauth, err := ioutil.ReadFile(ca)
	if err != nil {
		return fmt.Errorf("could not read ca certificate: %s", err)
	}

	if ok := certPool.AppendCertsFromPEM(cauth); !ok {
		return fmt.Errorf("failed to append client certs")
	}

	freePort, err := GetFreePort()
	if err != nil {
		log.Fatalf("failed to find a free port to start gRPC REST server: %s", err)
	}
	grpcAddress := fmt.Sprintf("%s:%d", "127.0.0.1", freePort)

	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("failed to start gRPC REST server: %s", err)
	}
	s := Server{Enclave : tm.Enclave}

	creds := credentials.NewTLS(&tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    certPool,
	})
	grpcServer := grpc.NewServer(grpc.Creds(creds))

	//creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	//if err != nil {
	//	log.Fatalf("failed to load credentials : %v", err)
	//}
	//
	//grpcServer := grpc.NewServer([]grpc.ServerOption{grpc.Creds(creds)}...)
	RegisterClientServer(grpcServer, &s)
	go func() {
		log.Fatal(grpcServer.Serve(lis))
	}()

	address := fmt.Sprintf("%s:%d", "127.0.0.1", port)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	mux := runtime.NewServeMux()
	err = RegisterClientHandlerFromEndpoint(ctx, mux, grpcAddress, []grpc.DialOption{grpc.WithTransportCredentials(creds)})
	if err != nil {
		log.Fatalf("could not register service : %s", err)
		return err
	}

	go func() {
		http.ListenAndServeTLS(address, certFile, keyFile, mux)
	}()
	log.Printf("started HTTPS REST server on %s", address)
	return nil
}

func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
