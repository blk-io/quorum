package main

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"log"
	"github.com/blk-io/crux/server"
	"encoding/base64"
	"fmt"
	"google.golang.org/grpc/credentials"
)

//const sender = "BULeR8JyUWhiuuCMU/HLA0Q5pzkYT+cHII3ZKBey3Bo="
const sender = "zSifTnkv5r4K67Dq304eVcM4FpxGfHLe1yTCBm0/7wg="
//const receiver = "QfeDAys9MPDs2XHExtc84jKGHxZg/aj52DTh0vtA3Xc="
const receiver = "a6AW4RRp+zmxeq9jg9fHdThv4wW1F1NiH1YAW4D1jVY="

var payload = []byte("payload")
var encodedPayload = base64.StdEncoding.EncodeToString(payload)

func main() {
	var conn *grpc.ClientConn
	ipcPath := "crux.ipc"
	creds, err := credentials.NewClientTLSFromFile("crux/cert/server.crt", "")
	if err != nil {
		log.Fatalf("could not load tls cert: %s", err)
	}
	conn, err = grpc.Dial(fmt.Sprintf("passthrough:///unix://%s", ipcPath), grpc.WithTransportCredentials(creds))
	//conn, err := grpc.Dial(fmt.Sprintf("passthrough:///unix://%s", ipcPath), grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %s", err)
	}
	defer conn.Close()
	c := server.NewClientClient(conn)
	sendReqs := []server.SendRequest{
		{
			Payload: payload,
			From: sender,
			To: []string{receiver},
		},
		{
			Payload: payload,
			To: []string{},
		},
		{
			Payload: payload,
		},
	}
	var testpayload []byte

	for _, sendReq := range sendReqs {
		tmanager := server.ApiVersion{}
		_, err := c.Version(context.Background(), &tmanager)

		if err != nil {
			log.Fatalf("Error when calling Crux Client: %s", err)
		}
		resp, err:= c.Send(context.Background(), &sendReq)
		if err != nil {
			log.Fatalf("grpc send error no resp")
		}
		resp1 := server.SendResponse{Key:resp.Key}
		if len(resp1.Key) == 0 || err != nil {
			log.Fatalf("grpc send error")
		}
		testpayload = resp1.Key
		log.Printf("Payload sent ")
	}

	log.Printf("Receive function called")
	receiveReqs := []server.ReceiveRequest{
		{
			Key: testpayload,
			To: receiver,
		},
	}

	for _, receiveReq := range receiveReqs {
		resp, err:= c.Receive(context.Background(), &receiveReq)
		if err != nil {
			log.Fatalf("gRPC receive failed with %s", err)
		}
		response := server.ReceiveResponse{Payload:resp.Payload}
		if len(response.Payload) == 0 || err != nil {
			log.Fatalf("grpc receive error")
		}
		log.Printf("Payload received ")
	}
}


