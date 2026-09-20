// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

// A small client for the BFBC2 remote administration (RCON) protocol.
//
// In plain words:
//   - We talk to the game server over TCP.
//   - Every message is a list of words ("serverInfo", "vars.hardCore", "true").
//   - To log in we ask for a salt, then send MD5(salt + password).
//   - The server answers with a list of words too. The first word is "OK" or an
//     error name, the rest is the data.
//
// One Client keeps one connection open and reconnects by itself if it breaks.
// Only one command runs at a time (the mutex), which is what the game expects.

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	flagResponse  = 0x40000000 // set on packets that answer one of our commands
	maxPacketSize = 1 << 20    // refuse anything bigger than 1 MiB
	dialTimeout   = 3 * time.Second
	ioTimeout     = 5 * time.Second
)

// CmdError is returned when the server understood us but refused the command
// (for example "InvalidArguments"). The connection is fine.
type CmdError struct{ Status string }

func (e *CmdError) Error() string { return "server answered: " + e.Status }

// ErrLogin means the RCON password was not accepted.
var ErrLogin = errors.New("RCON login failed (wrong password?)")

type Client struct {
	addr     string
	password string

	mu   sync.Mutex
	conn net.Conn
	seq  uint32
}

func NewClient(addr, password string) *Client {
	return &Client{addr: addr, password: password}
}

// Do sends one command and returns the words after the leading "OK".
func (c *Client) Do(words ...string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if c.conn == nil {
			if err := c.connect(); err != nil {
				lastErr = err
				if errors.Is(err, ErrLogin) {
					return nil, err // no point in retrying a wrong password
				}
				continue
			}
		}
		resp, err := c.roundTrip(words)
		if err != nil {
			c.closeLocked() // network problem: reconnect and try once more
			lastErr = err
			continue
		}
		if len(resp) == 0 {
			return nil, &CmdError{Status: "empty answer"}
		}
		if resp[0] != "OK" {
			return nil, &CmdError{Status: resp[0]}
		}
		return resp[1:], nil
	}
	return nil, lastErr
}

// Close drops the connection (a new one is opened on the next command).
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}

func (c *Client) closeLocked() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func (c *Client) connect() error {
	conn, err := net.DialTimeout("tcp", c.addr, dialTimeout)
	if err != nil {
		return err
	}
	c.conn = conn
	c.seq = 0

	resp, err := c.roundTrip([]string{"login.hashed"})
	if err != nil {
		c.closeLocked()
		return err
	}
	if len(resp) < 2 || resp[0] != "OK" {
		c.closeLocked()
		return fmt.Errorf("unexpected answer to login request: %v", resp)
	}
	salt, err := hex.DecodeString(resp[1])
	if err != nil {
		c.closeLocked()
		return fmt.Errorf("bad salt from server: %w", err)
	}
	sum := md5.Sum(append(salt, []byte(c.password)...))
	digest := strings.ToUpper(hex.EncodeToString(sum[:]))

	resp, err = c.roundTrip([]string{"login.hashed", digest})
	if err != nil {
		c.closeLocked()
		return err
	}
	if len(resp) == 0 || resp[0] != "OK" {
		c.closeLocked()
		return ErrLogin
	}
	return nil
}

func (c *Client) roundTrip(words []string) ([]string, error) {
	c.conn.SetDeadline(time.Now().Add(ioTimeout))
	if _, err := c.conn.Write(encodePacket(c.seq, words)); err != nil {
		return nil, err
	}
	c.seq = (c.seq + 1) & 0x3fffffff

	// Skip anything that is not an answer (the server may send event packets).
	for {
		header, resp, err := readPacket(c.conn)
		if err != nil {
			return nil, err
		}
		if header&flagResponse != 0 {
			return resp, nil
		}
	}
}

// encodePacket builds one packet: header, total size, word count, then words.
// Each word is: length (4 bytes), the text, and a zero byte.
func encodePacket(seq uint32, words []string) []byte {
	size := 12
	for _, w := range words {
		size += 4 + len(w) + 1
	}
	buf := make([]byte, 0, size)
	buf = binary.LittleEndian.AppendUint32(buf, seq&0x3fffffff)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(size))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(words)))
	for _, w := range words {
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(w)))
		buf = append(buf, w...)
		buf = append(buf, 0)
	}
	return buf
}

// readPacket reads one packet and returns its header and its words.
func readPacket(r io.Reader) (uint32, []string, error) {
	head := make([]byte, 12)
	if _, err := io.ReadFull(r, head); err != nil {
		return 0, nil, err
	}
	header := binary.LittleEndian.Uint32(head[0:4])
	size := binary.LittleEndian.Uint32(head[4:8])
	count := binary.LittleEndian.Uint32(head[8:12])
	if size < 12 || size > maxPacketSize {
		return 0, nil, fmt.Errorf("bad packet size %d", size)
	}
	body := make([]byte, size-12)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	words := make([]string, 0, count)
	off := 0
	for i := uint32(0); i < count; i++ {
		if off+4 > len(body) {
			return 0, nil, errors.New("truncated packet")
		}
		n := int(binary.LittleEndian.Uint32(body[off : off+4]))
		if n < 0 || off+4+n+1 > len(body) {
			return 0, nil, errors.New("truncated word")
		}
		words = append(words, string(body[off+4:off+4+n]))
		off += 4 + n + 1
	}
	return header, words, nil
}
