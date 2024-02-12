package main_test

import (
	"context"
	"errors"
	"io"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func Test_integration_RANP(t *testing.T) {
	tests := []struct {
		name    string
		cmdArgs []string
	}{
		{
			name:    "python",
			cmdArgs: []string{"python", "-OO", "retroarch_tunnel_server.py"},
		},
		{
			name:    "go",
			cmdArgs: []string{"go", "run", "main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cmd := exec.CommandContext(ctx, tt.cmdArgs[0], tt.cmdArgs[1:]...)
			go cmd.Run()
			time.Sleep(time.Second)

			var conn net.Conn
			var err error

			for i := 1; i <= 5; i++ {
				conn, err = net.Dial("tcp", ":55435")
				if err == nil {
					break
				}
				time.Sleep(time.Duration(i) * 300 * time.Millisecond)
			}

			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			_, err = conn.Write([]byte("RANP"))
			if err != nil {
				t.Fatal(err)
			}

			buf := make([]byte, 24)
			_, err = io.ReadFull(conn, buf)
			if err != nil {
				t.Fatal(err)
			}

			want := "RANP" + strings.Repeat("\x00", 20)
			got := string(buf)

			if want != got {
				t.Fatalf("want: %s, got: %s", want, got)
			}

			_, err = conn.Read([]byte{0})
			if !errors.Is(err, io.EOF) {
				t.Fatalf("want: %v, got: %v", io.EOF, err)
			}
		})
	}
}
