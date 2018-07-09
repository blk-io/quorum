package constellation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tv42/httpunix"
	"github.com/blk-io/crux/server"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
	"log"
)

func launchNode(cfgPath string) (*exec.Cmd, error) {
	cmd := exec.Command("constellation-node", cfgPath)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	go io.Copy(os.Stderr, stderr)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	time.Sleep(100 * time.Millisecond)
	return cmd, nil
}

func unixTransport(socketPath string) *httpunix.Transport {
	t := &httpunix.Transport{
		DialTimeout:           1 * time.Second,
		RequestTimeout:        5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	t.RegisterLocation("c", socketPath)
	return t
}

func createNewClient(socketPath string, grpc bool) *Client {
	if grpc{
		return grpcClient(socketPath)
	}
	return unixClient(socketPath)
}
func unixClient(socketPath string) *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: unixTransport(socketPath),
		},
	}
}

func grpcClient(socketPath string) *Client{
	var conn *grpc.ClientConn
	conn, err := grpc.Dial(fmt.Sprintf("passthrough:///unix://%s", socketPath), grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %s", err)
	}
	defer conn.Close()
	var client Client
	client.grpcClient = server.NewClientClient(conn)
	return &client
}

func UpCheck(socketPath string, grpc bool) error {
	if grpc {
		_ = grpcClient(socketPath)
		return RunNodeGrpc(socketPath)
	}
	return RunNodeUnix(socketPath)
}

func RunNodeUnix(socketPath string) error {
	c := unixClient(socketPath).httpClient
	res, err := c.Get("http+unix://c/upcheck")
	if err != nil {
		return err
	}
	if res.StatusCode == 200 {
		return nil
	}
	return errors.New("Constellation Node API did not respond to upcheck request")
}

func RunNodeGrpc(socketPath string) error {
	c := grpcClient(socketPath).grpcClient
	uc := server.UpCheckResponse{}
	_, err := c.Upcheck(context.Background(), &uc)
	if err != nil {
		return errors.New("Constellation Node gRPC API did not respond to upcheck request")
	}
	return nil
}

type Client struct {
	httpClient *http.Client
	grpcClient server.ClientClient
	usegrpc bool
}

func (c *Client) doJson(path string, apiReq interface{}) (*http.Response, error) {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(apiReq)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "http+unix://c/"+path, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err == nil && res.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}
	return res, err
}

func (c *Client) SendPayloadGrpc(pl []byte, b64From string, b64To []string) ([]byte, error){
	resp, err := c.grpcClient.Send(context.Background(), &server.SendRequest{Payload: pl, From: b64From,To: b64To})
	if err != nil {
		return nil, fmt.Errorf("Send Payload failed: %v", err)
	}
	return resp.Key, nil
}

func (c *Client) SendPayload(pl []byte, b64From string, b64To []string) ([]byte, error) {
	buf := bytes.NewBuffer(pl)
	req, err := http.NewRequest("POST", "http+unix://c/sendraw", buf)
	if err != nil {
		return nil, err
	}
	if b64From != "" {
		req.Header.Set("c11n-from", b64From)
	}
	req.Header.Set("c11n-to", strings.Join(b64To, ","))
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := c.httpClient.Do(req)
	if err == nil && res.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}
	defer res.Body.Close()
	return ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, res.Body))
}

func (c *Client) ReceivePayloadGrpc(data []byte) ([]byte, interface{}) {
	resp, err := c.grpcClient.Receive(context.Background(), &server.ReceiveRequest{Key: data})
	if err != nil {
		return nil, fmt.Errorf("Receive Payload failed: %v", err)
	}
	return resp.Payload, nil
}

func (c *Client) ReceivePayload(key []byte) ([]byte, error) {
	req, err := http.NewRequest("GET", "http+unix://c/receiveraw", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("c11n-key", base64.StdEncoding.EncodeToString(key))
	res, err := c.httpClient.Do(req)
	if err == nil && res.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 status code: %+v", res)
	}
	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}

func NewClient(socketPath string, grpc bool) (*Client, error) {
	return createNewClient(socketPath, grpc), nil
}
