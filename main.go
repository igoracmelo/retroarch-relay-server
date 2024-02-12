package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

func main() {
	logFile, err := os.Open("gotunnel.log")
	if err != nil {
		log.Fatal(err)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(
		io.MultiWriter(os.Stderr, logFile),
		&slog.HandlerOptions{
			Level:     slog.LevelDebug,
			AddSource: true,
		},
	)))

	err = run(55435)
	if err != nil {
		log.Fatal(err)
	}
}

func run(port uint16) error {
	l, err := net.Listen("tcp", ":"+fmt.Sprint(port))
	if err != nil {
		return err
	}
	slog.Info("listening on ::|" + fmt.Sprint(port))
	s := &server{
		mut: &sync.Mutex{},
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		slog.Info("accepted connection", "addr", conn.RemoteAddr())

		go func() {
			err := s.handle(conn)
			if err != nil {
				slog.Error("error", "err", err)
			}
		}()
	}
}

type client struct {
	conn        net.Conn
	unique      []byte
	onwerID     []byte
	isOnwer     bool
	isReady     bool
	isConnected bool
}

type server struct {
	mut     *sync.Mutex
	clients []client
}

func (s *server) handle(conn net.Conn) error {
	bMagic := make([]byte, 4)
	err := withDeadline(conn, time.Now().Add(30*time.Second), func() error {
		_, err := io.ReadFull(conn, bMagic)
		return err
	})
	if err != nil {
		return err
	}
	magic := string(bMagic)

	if magic == "RANP" {
		defer conn.Close()
		defer slog.Error("Unsupported client from", "addr", conn.RemoteAddr().String())
		s := "RANP" + strings.Repeat("\x00", 20)
		_, err = conn.Write([]byte(s))
		return err
	}

	if magic == "RATS" || magic == "RATL" {
		unique := make([]byte, 12)
		zero := make([]byte, 12)

		err = withDeadline(conn, time.Now().Add(30*time.Second), func() error {
			_, err = io.ReadFull(conn, unique)
			return err
		})
		if err != nil {
			slog.Error("failed to read unique", "addr", conn.RemoteAddr(), "magic", magic)
			return err
		}

		if magic == "RATS" {
			if bytes.Equal(unique, zero) {
				// create new session

				// TODO: limit max sessions

				// new client is session owner

				s.addClient(client{})

			} else {
				// unique != zero, trying to link to a session
				sessionID := make([]byte, 12)
				copy(sessionID, unique)

				s.addClient(client{})

				// link client to session
				panic("todo")

				s.linkClientToSession(client{}, sessionID)
			}
		}
	}

	return nil
}

func (s *server) addClient(c client) {
	s.mut.Lock()
	defer s.mut.Unlock()

	unique := make([]byte, 12)
	for {
		generateUnique(unique)
		exists := slices.ContainsFunc(s.clients, func(c client) bool {
			return slices.Equal(c.unique, unique)
		})
		if !exists {
			break
		}
	}

	c.unique = unique
	s.clients = append(s.clients, c)
}

func (s *server) linkClientToSession(cl client, sessionID []byte) (*client, error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	if slices.Equal(cl.unique, sessionID) {
		return nil, errors.New("client tried to link to himself")
	}

	i := slices.IndexFunc(s.clients, func(c client) bool {
		return slices.Equal(c.unique, sessionID)
	})
	if i == -1 {
		return nil, errors.New("session not found")
	}

	owner := s.clients[i]
	if !owner.isOnwer {
		return nil, errors.New("not a owner")
	}

	i = slices.IndexFunc(s.clients, func(c client) bool {
		return slices.Equal(c.unique, cl.unique)
	})
	s.clients[i].onwerID = owner.unique

	if !owner.isReady {
		return nil, errors.New("owner not ready")
	}

	// TODO check if connection is open
	// TODO max connections per session

	if cl.isConnected {
		return nil, errors.New("client already connected")
	}

	msg := "RATL" + string(cl.unique)
	err := withDeadline(owner.conn, time.Now().Add(30*time.Second), func() error {
		_, err := owner.conn.Write([]byte(msg))
		return err
	})
	if err != nil {
		return nil, err
	}

	s.clients[i].isConnected = true

	return &owner, nil
}

// async def __session_link_request(self, user: SessionUserClient, session_id: bytes) -> Optional[SessionOwner]:
//     except KeyError:
//         return await self.log_error(f"Failed to find session for: {user.address}|{user.port}")
//     else:
//         await user.set_owner(owner)

//         if not await owner.request_link(user):
//             return await self.log_error(f"Failed to establish tunnel link for: {user.address}|{user.port}")

//     return owner

func generateUnique(b []byte) {
	if len(b) != 12 {
		panic("wrong length")
	}

	zero := make([]byte, 12)
	for {
		_, err := rand.Read(b)
		if err != nil {
			slog.Error("rand.Read", "err", err)
			continue
		}
		if bytes.Equal(b, zero) {
			continue
		}
	}
}

func withDeadline(conn net.Conn, t time.Time, f func() error) (err error) {
	err = conn.SetDeadline(t)
	if err != nil {
		return
	}

	defer func() {
		err2 := conn.SetDeadline(time.Time{})
		err = errors.Join(err, err2)
	}()

	return f()
}
