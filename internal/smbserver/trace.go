package smbserver

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

var commands = []string{"NEGOTIATE", "SESSION_SETUP", "LOGOFF", "TREE_CONNECT", "TREE_DISCONNECT", "CREATE", "CLOSE", "FLUSH", "READ", "WRITE", "LOCK", "IOCTL", "CANCEL", "ECHO", "QUERY_DIRECTORY", "CHANGE_NOTIFY", "QUERY_INFO", "SET_INFO", "OPLOCK_BREAK"}

func command(n uint16) string {
	if int(n) < len(commands) {
		return commands[n]
	}
	return fmt.Sprintf("COMMAND_%d", n)
}
func dialect(n uint16) string {
	switch n {
	case 0x0202:
		return "SMB2.0.2"
	case 0x0210:
		return "SMB2.1"
	case 0x0300:
		return "SMB3.0"
	case 0x0302:
		return "SMB3.0.2"
	case 0x0311:
		return "SMB3.1.1"
	case 0x02ff:
		return "SMB2_WILDCARD"
	}
	return fmt.Sprintf("0x%04x", n)
}
func statusName(n uint32) string {
	switch n {
	case 0:
		return "SUCCESS"
	case 0x103:
		return "PENDING"
	case 0xc0000016:
		return "MORE_PROCESSING_REQUIRED"
	case 0x80000005:
		return "BUFFER_OVERFLOW"
	case 0x80000006:
		return "NO_MORE_FILES"
	case 0xc0000011:
		return "END_OF_FILE"
	case 0xc0000022:
		return "ACCESS_DENIED"
	case 0xc000006d:
		return "LOGON_FAILURE"
	case 0xc00000bb:
		return "NOT_SUPPORTED"
	case 0xc0000002:
		return "NOT_IMPLEMENTED"
	case 0xc0000034:
		return "OBJECT_NAME_NOT_FOUND"
	case 0xc00000cc:
		return "BAD_NETWORK_NAME"
	case 0xc00000a2:
		return "MEDIA_WRITE_PROTECTED"
	}
	return fmt.Sprintf("0x%08x", n)
}

// frameTap handles split TCP reads and coalesced frames. It keeps only the
// first 4 KiB; video data and authentication tokens are NEVER logged.
type frameTap struct {
	header            [4]byte
	have              int
	remaining, length int
	prefix            []byte
	disabled          bool
	onFrame           func([]byte, int)
}

func (t *frameTap) feed(p []byte) {
	for len(p) > 0 && !t.disabled {
		if t.have < 4 {
			n := copy(t.header[t.have:], p)
			t.have += n
			p = p[n:]
			if t.have < 4 {
				return
			}
			t.length = int(binary.BigEndian.Uint32(t.header[:]) & 0xffffff)
			if t.header[0] != 0 || t.length == 0 || t.length > 8<<20 {
				t.disabled = true
				t.onFrame(nil, t.length)
				return
			}
			t.remaining = t.length
		}
		n := min(t.remaining, len(p))
		keep := min(n, 4096-len(t.prefix))
		t.prefix = append(t.prefix, p[:keep]...)
		p = p[n:]
		t.remaining -= n
		if t.remaining == 0 {
			t.onFrame(t.prefix, t.length)
			clear(t.prefix)
			t.prefix = t.prefix[:0]
			t.have = 0
		}
	}
}

type traceConn struct {
	net.Conn
	id                     uint64
	log                    *eventLog
	mu                     sync.Mutex
	writeMu                sync.Mutex
	closed                 bool
	in, out                frameTap
	requests, responses    map[string]uint64
	statuses               map[string]uint64
	authOK, treeOK, listOK uint64
	readBytes              uint64
	selected               string
	smb1Only               bool
	release                func()
}

func newTraceConn(c net.Conn, id uint64, l *eventLog) *traceConn {
	t := &traceConn{Conn: c, id: id, log: l, requests: map[string]uint64{}, responses: map[string]uint64{}, statuses: map[string]uint64{}}
	t.in.onFrame = func(b []byte, n int) { t.observe(b, n, false) }
	t.out.onFrame = func(b []byte, n int) { t.observe(b, n, true) }
	return t
}
func (c *traceConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	authenticated := c.authOK > 0
	c.mu.Unlock()
	if authenticated {
		c.Conn.SetReadDeadline(time.Time{})
	} else {
		c.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	}
	n, err := c.Conn.Read(p)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.in.feed(p[:n])
		if err != nil {
			c.log.event("read_end", map[string]any{"conn": c.id, "reason": err.Error()})
		}
	}
	return n, err
}
func (c *traceConn) Write(p []byte) (int, error) {
	// Keep outbound observations in TCP order, including async SMB replies.
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	n, err := c.Conn.Write(p)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.out.feed(p[:n])
		if err != nil {
			c.log.event("write_end", map[string]any{"conn": c.id, "reason": err.Error()})
		}
	}
	return n, err
}
func (c *traceConn) Close() error {
	err := c.Conn.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		stage := "connected_only"
		if c.smb1Only {
			stage = "client_offered_only_SMB1"
		}
		if c.selected != "" && c.selected != "SMB2_WILDCARD" {
			stage = "protocol_negotiated"
		}
		if c.authOK > 0 {
			stage = "authenticated"
		}
		if c.treeOK > 0 {
			stage = "tree_connected_including_possible_IPC"
		}
		if c.listOK > 0 {
			stage = "directory_listed"
		}
		if c.readBytes > 0 {
			stage = "SMB_read_succeeded_may_include_RPC"
		}
		c.log.event("connection_summary", map[string]any{"conn": c.id, "stage": stage, "dialect": c.selected, "smb1_only_offer": c.smb1Only, "authenticated": c.authOK, "tree_connect_successes": c.treeOK, "directory_list_successes": c.listOK, "smb_read_bytes": c.readBytes, "requests": c.requests, "responses": c.responses, "statuses": c.statuses})
		if c.release != nil {
			c.release()
		}
	}
	return err
}

func (c *traceConn) observe(p []byte, total int, response bool) {
	if len(p) < 4 {
		c.log.event("unrecognized_frame", map[string]any{"conn": c.id, "bytes": total, "response": response})
		return
	}
	if bytes.Equal(p[:4], []byte{0xff, 'S', 'M', 'B'}) {
		if !response && len(p) >= 35 && p[4] == 0x72 {
			var offered []string
			start := 33 + int(p[32])*2
			if start+2 <= len(p) {
				n := int(binary.LittleEndian.Uint16(p[start:]))
				start += 2
				data := p[start:min(start+n, len(p))]
				for len(data) > 1 && len(offered) < 32 {
					if data[0] != 2 {
						break
					}
					end := bytes.IndexByte(data[1:], 0)
					if end < 0 {
						break
					}
					s := string(data[1 : 1+end])
					if len(s) > 80 {
						s = s[:80]
					}
					offered = append(offered, s)
					data = data[end+2:]
				}
			}
			modern := false
			for _, s := range offered {
				if strings.HasPrefix(s, "SMB 2.") {
					modern = true
				}
			}
			c.smb1Only = len(offered) > 0 && !modern
			c.log.event("legacy_negotiate", map[string]any{"conn": c.id, "offered": offered, "smb1_only_offer": c.smb1Only})
		} else {
			c.log.event("smb1_frame", map[string]any{"conn": c.id, "response": response})
		}
		return
	}
	if !bytes.Equal(p[:4], []byte{0xfe, 'S', 'M', 'B'}) {
		c.log.event("non_smb2_frame", map[string]any{"conn": c.id, "bytes": total})
		return
	}
	for len(p) >= 64 {
		next := int(binary.LittleEndian.Uint32(p[20:]))
		one := p
		if next > 0 && next <= len(p) {
			one = p[:next]
		}
		c.observeSMB2(one, response)
		if next == 0 {
			break
		}
		if next < 64 || next > len(p) {
			c.log.event("compound_trace_truncated", map[string]any{"conn": c.id, "next_offset": next})
			break
		}
		p = p[next:]
	}
}

func (c *traceConn) observeSMB2(p []byte, response bool) {
	cmd := binary.LittleEndian.Uint16(p[12:])
	name := command(cmd)
	flags := binary.LittleEndian.Uint32(p[16:])
	st := binary.LittleEndian.Uint32(p[8:])
	e := map[string]any{"conn": c.id, "command": name, "message_id": binary.LittleEndian.Uint64(p[24:]), "signed": flags&8 != 0}
	body := p[64:]
	kind := "smb_request"
	if response {
		kind = "smb_response"
		c.responses[name]++
		c.statuses[statusName(st)]++
		e["status"], e["status_hex"] = statusName(st), fmt.Sprintf("0x%08x", st)
		if st == 0 {
			switch cmd {
			case 0:
				if len(body) >= 6 {
					c.selected = dialect(binary.LittleEndian.Uint16(body[4:]))
					e["dialect"] = c.selected
					e["security_mode"] = binary.LittleEndian.Uint16(body[2:])
					kind = "negotiated"
				}
			case 1:
				c.authOK++
				kind = "authentication_ok"
			case 3:
				c.treeOK++
				kind = "tree_connected"
			case 14:
				c.listOK++
			case 8:
				if len(body) >= 8 {
					n := binary.LittleEndian.Uint32(body[4:])
					c.readBytes += uint64(n)
					e["bytes"] = n
				}
			}
		} else if cmd == 1 && st != 0xc0000016 {
			kind = "authentication_failed"
		}
	} else {
		c.requests[name]++
		switch cmd {
		case 0:
			if len(body) >= 36 {
				var ds []string
				count := min(int(binary.LittleEndian.Uint16(body[2:])), (len(body)-36)/2)
				for i := 0; i < count; i++ {
					ds = append(ds, dialect(binary.LittleEndian.Uint16(body[36+i*2:])))
				}
				e["dialects"], e["security_mode"], kind = ds, binary.LittleEndian.Uint16(body[4:]), "negotiate_offer"
			}
		case 1:
			if len(body) >= 16 {
				o, n := int(binary.LittleEndian.Uint16(body[12:])), int(binary.LittleEndian.Uint16(body[14:]))
				if token := region(p, o, n); token != nil {
					i := bytes.Index(token, []byte("NTLMSSP\x00"))
					if i >= 0 && i+12 <= len(token) {
						e["ntlm_message_type"] = binary.LittleEndian.Uint32(token[i+8:])
					}
				}
			}
		case 3:
			if len(body) >= 8 {
				o, n := int(binary.LittleEndian.Uint16(body[4:])), int(binary.LittleEndian.Uint16(body[6:]))
				s := utf16String(region(p, o, n))
				parts := strings.Split(strings.TrimRight(s, "\\"), "\\")
				if len(parts) > 0 {
					last := parts[len(parts)-1]
					if strings.EqualFold(last, "Media") || strings.EqualFold(last, "IPC$") {
						e["share"] = last
					} else {
						e["share_id"] = pathID(last)
					}
				}
			}
		case 5:
			if len(body) >= 48 {
				o, n := int(binary.LittleEndian.Uint16(body[44:])), int(binary.LittleEndian.Uint16(body[46:]))
				s := utf16String(region(p, o, n))
				e["path_id"] = pathID(s)
				e["name_utf16_bytes"] = n
				e["disposition"] = binary.LittleEndian.Uint32(body[36:])
				if strings.EqualFold(strings.Trim(s, "\\"), "srvsvc") {
					e["pipe"] = "srvsvc"
				}
			}
		case 8:
			if len(body) >= 16 {
				e["length"], e["offset"] = binary.LittleEndian.Uint32(body[4:]), binary.LittleEndian.Uint64(body[8:])
			}
		case 11:
			if len(body) >= 32 {
				e["ctl_code"] = fmt.Sprintf("0x%08x", binary.LittleEndian.Uint32(body[4:]))
				o, n := int(binary.LittleEndian.Uint32(body[24:])), int(binary.LittleEndian.Uint32(body[28:]))
				if b := region(p, o, n); len(b) >= 24 && b[0] == 5 {
					e["rpc_type"] = b[2]
					if b[2] == 0 {
						var op uint16
						if b[4]&0x10 != 0 {
							op = binary.LittleEndian.Uint16(b[22:])
						} else {
							op = binary.BigEndian.Uint16(b[22:])
						}
						e["rpc_opnum"] = op
						if op == 15 {
							kind = "share_enum_request"
						}
					}
				}
			}
		case 14:
			if len(body) >= 3 {
				e["info_class"] = body[2]
			}
		}
	}
	// Keep startup/control traffic and failures; sample successful bulk traffic.
	count := c.requests[name]
	if response {
		count = c.responses[name]
	}
	bulk := cmd == 8 || cmd == 6 || cmd == 13 || cmd == 16
	if bulk && count > 8 && count%256 != 0 && (!response || st == 0) {
		return
	}
	c.log.event(kind, e)
}

func region(b []byte, o, n int) []byte {
	if o < 64 || n < 0 || o > len(b) || n > len(b)-o {
		return nil
	}
	return b[o : o+n]
}
func utf16String(b []byte) string {
	a := make([]uint16, len(b)/2)
	for i := range a {
		a[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(a))
}
