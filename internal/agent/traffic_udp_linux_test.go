package agent

import (
	"net"
	"testing"
	"time"
)

func TestPassiveUDPAccounting(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	collector := startUDPTraffic(server.LocalAddr().(*net.UDPAddr).Port)
	if collector == nil {
		t.Skip("CAP_NET_RAW is required for passive UDP accounting")
	}
	defer collector.Close()
	client, err := net.DialUDP("udp4", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Write([]byte("upload-test")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 100)
	_ = server.SetReadDeadline(time.Now().Add(time.Second))
	n, from, err := server.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = server.WriteToUDP(buf[:n], from); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		v := collector.Sample()
		if v != nil && v.UploadBytes == uint64(n) && v.DownloadBytes == uint64(n) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected exactly one UDP payload each direction, got %+v", collector.Sample())
}
