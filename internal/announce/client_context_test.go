package announce_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"gotorrent/internal/announce"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type requestBody struct {
	ctx  context.Context
	data *strings.Reader
}

func (b *requestBody) Read(p []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	return b.data.Read(p)
}

func (b *requestBody) Close() error { return nil }

func TestClientDoKeepsContextUntilResponseBodyRead(t *testing.T) {
	const body = "{\"interval\":17,\"complete\":1,\"incomplete\":2,\"peers\":[{\"peer_id\":\"peer-b\",\"ip\":\"10.0.0.8\",\"port\":6881}]}"

	client := announce.NewClient()
	client.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: &requestBody{
				ctx:  req.Context(),
				data: strings.NewReader(body),
			},
			Request: req,
		}, nil
	})}

	resp, err := client.Do(announce.Request{
		AnnounceURL: "http://tracker.example/announce",
		InfoHash:    "0123456789012345678901234567890123456789",
		PeerID:      "-GT0001-abcdefghijkl",
		Port:        6881,
		Left:        1024,
		Event:       announce.EventStarted,
	})
	if err != nil {
		t.Fatalf("读取合法 announce 响应失败: %v", err)
	}
	if resp.Interval != 17 || len(resp.Peers) != 1 || resp.Peers[0].IP != "10.0.0.8" {
		t.Fatalf("announce 响应异常: %+v", resp)
	}
}
