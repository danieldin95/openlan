package libol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/xtaci/kcp-go/v5"
	"net"
	"time"
)

const (
	MAXBUF = 4096
	HSIZE  = 0x04
)

var MAGIC = []byte{0xff, 0xff}

func isControl(data []byte) bool {
	if len(data) < 6 {
		return false
	}
	if bytes.Equal(data[:6], ZEROED[:6]) {
		return true
	}
	return false
}

type Ip4Protocol struct {
	Err  error
	Eth  *Ether
	Vlan *Vlan
	Arp  *Arp
	Ip4  *Ipv4
	Udp  *Udp
	Tcp  *Tcp
}

type FrameMessage struct {
	control bool
	action  string
	params  string
	buffer  []byte
	size    int
	total   int
	frame   []byte
	proto   *Ip4Protocol
}

func NewFrameMessage() *FrameMessage {
	m := FrameMessage{
		control: false,
		action:  "",
		params:  "",
		size:    0,
		buffer:  make([]byte, HSIZE+MAXBUF),
	}
	m.frame = m.buffer[HSIZE:]
	m.total = len(m.frame)
	return &m
}

func (m *FrameMessage) Decode() bool {
	m.control = isControl(m.frame)
	if m.control {
		m.action = string(m.frame[6:11])
		m.params = string(m.frame[12:])
	}
	return m.control
}

func (m *FrameMessage) IsControl() bool {
	return m.control
}

func (m *FrameMessage) Frame() []byte {
	return m.frame
}

func (m *FrameMessage) String() string {
	return fmt.Sprintf("control: %t, frame: %x", m.control, m.frame[:20])
}

func (m *FrameMessage) CmdAndParams() (string, string) {
	return m.action, m.params
}

func (m *FrameMessage) Append(data []byte) {
	add := len(data)
	if m.total-m.size >= add {
		copy(m.frame[m.size:], data)
		m.size += add
	}
}

func (m *FrameMessage) Size() int {
	return m.size
}

func (m *FrameMessage) SetSize(v int) {
	m.size = v
}

func (m *FrameMessage) Proto() (*Ip4Protocol, error) {
	if m.proto != nil {
		return m.proto, m.proto.Err
	}
	data := m.frame
	p := new(Ip4Protocol)
	if p.Eth, p.Err = NewEtherFromFrame(data); p.Err != nil {
		return nil, p.Err
	}
	data = data[p.Eth.Len:]
	if p.Eth.IsVlan() {
		if p.Vlan, p.Err = NewVlanFromFrame(data); p.Err != nil {
			return nil, p.Err
		}
		data = data[p.Vlan.Len:]
	}
	if p.Eth.IsIP4() {
		if p.Ip4, p.Err = NewIpv4FromFrame(data); p.Err != nil {
			return nil, p.Err
		}
		data = data[p.Ip4.Len:]
		switch p.Ip4.Protocol {
		case IpTcp:
			if p.Tcp, p.Err = NewTcpFromFrame(data); p.Err != nil {
				return nil, p.Err
			}
		case IpUdp:
			if p.Udp, p.Err = NewUdpFromFrame(data); p.Err != nil {
				return nil, p.Err
			}
		}
	} else if p.Eth.IsArp() {
		if p.Arp, p.Err = NewArpFromFrame(data); p.Err != nil {
			return nil, p.Err
		}
	}
	m.proto = p
	return m.proto, m.proto.Err
}

type ControlMessage struct {
	control  bool
	operator string
	action   string
	params   string
}

//operator: request is '= ', and response is  ': '
//action: login, network etc.
//body: json string.
func NewControlMessage(action string, opr string, body string) *ControlMessage {
	c := ControlMessage{
		control:  true,
		action:   action,
		params:   body,
		operator: opr,
	}
	return &c
}

func (c *ControlMessage) Encode() *FrameMessage {
	p := fmt.Sprintf("%s%s%s", c.action[:4], c.operator[:2], c.params)
	frame := NewFrameMessage()
	frame.Append(ZEROED[:6])
	frame.Append([]byte(p))
	return frame
}

type Messager interface {
	Send(conn net.Conn, frame *FrameMessage) (int, error)
	Receive(conn net.Conn, max, min int) (*FrameMessage, error)
}

type StreamMessage struct {
	timeout time.Duration // ns for read and write deadline.
	block   kcp.BlockCrypt
}

func (s *StreamMessage) write(conn net.Conn, tmp []byte) (int, error) {
	if s.timeout != 0 {
		err := conn.SetWriteDeadline(time.Now().Add(s.timeout))
		if err != nil {
			return 0, err
		}
	}
	n, err := conn.Write(tmp)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *StreamMessage) writeFull(conn net.Conn, buf []byte) error {
	if conn == nil {
		return NewErr("connection is nil")
	}
	offset := 0
	size := len(buf)
	left := size - offset
	Log("writeFull: %s %d", conn.RemoteAddr(), size)
	Log("writeFull: %s Data %x", conn.RemoteAddr(), buf)
	for left > 0 {
		tmp := buf[offset:]
		Log("writeFull: tmp %s %d", conn.RemoteAddr(), len(tmp))
		n, err := s.write(conn, tmp)
		if err != nil {
			return err
		}
		Log("writeFull: %s snd %d, size %d", conn.RemoteAddr(), n, size)
		offset += n
		left = size - offset
	}
	return nil
}

func (s *StreamMessage) Send(conn net.Conn, frame *FrameMessage) (int, error) {
	frame.buffer[0] = MAGIC[0]
	frame.buffer[1] = MAGIC[1]
	binary.BigEndian.PutUint16(frame.buffer[2:4], uint16(frame.size))
	if s.block != nil {
		s.block.Encrypt(frame.frame, frame.frame)
	}
	if err := s.writeFull(conn, frame.buffer[:frame.size+4]); err != nil {
		return 0, err
	}
	return frame.size, nil
}

func (s *StreamMessage) read(conn net.Conn, tmp []byte) (int, error) {
	if s.timeout != 0 {
		err := conn.SetReadDeadline(time.Now().Add(s.timeout))
		if err != nil {
			return 0, err
		}
	}
	n, err := conn.Read(tmp)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *StreamMessage) readFull(conn net.Conn, buf []byte) error {
	if conn == nil {
		return NewErr("connection is nil")
	}
	offset := 0
	left := len(buf)
	Log("readFull: %s %d", conn.RemoteAddr(), len(buf))
	for left > 0 {
		tmp := make([]byte, left)
		n, err := s.read(conn, tmp)
		if err != nil {
			return err
		}
		copy(buf[offset:], tmp)
		offset += n
		left -= n
	}
	Log("readFull: Data %s %x", conn.RemoteAddr(), buf)
	return nil
}

func (s *StreamMessage) Receive(conn net.Conn, max, min int) (*FrameMessage, error) {
	frame := NewFrameMessage()
	h := frame.buffer[:4]
	if err := s.readFull(conn, h); err != nil {
		return nil, err
	}
	if !bytes.Equal(h[:2], MAGIC[:2]) {
		return nil, NewErr("%s: wrong magic", conn.RemoteAddr())
	}
	size := int(binary.BigEndian.Uint16(h[2:4]))
	if size > max || size < min {
		return nil, NewErr("%s: wrong size %d", conn.RemoteAddr(), size)
	}
	tmp := frame.buffer[4 : 4+size]
	if err := s.readFull(conn, tmp); err != nil {
		return nil, err
	}
	if s.block != nil {
		s.block.Decrypt(tmp, tmp)
	}
	frame.size = size
	frame.frame = tmp
	return frame, nil
}

type DataGramMessage struct {
	timeout time.Duration // ns for read and write deadline
	block   kcp.BlockCrypt
}

func (s *DataGramMessage) Send(conn net.Conn, frame *FrameMessage) (int, error) {
	frame.buffer[0] = MAGIC[0]
	frame.buffer[1] = MAGIC[1]
	binary.BigEndian.PutUint16(frame.buffer[2:4], uint16(frame.size))
	if s.block != nil {
		s.block.Encrypt(frame.frame, frame.frame)
	}
	Log("DataGramMessage.Send: %s %x", conn.RemoteAddr(), frame)
	if s.timeout != 0 {
		err := conn.SetWriteDeadline(time.Now().Add(s.timeout))
		if err != nil {
			return 0, err
		}
	}
	if _, err := conn.Write(frame.buffer[:4+frame.size]); err != nil {
		return 0, err
	}
	return frame.size, nil
}

func (s *DataGramMessage) Receive(conn net.Conn, max, min int) (*FrameMessage, error) {
	frame := NewFrameMessage()
	Debug("DataGramMessage.Receive %s %d", conn.RemoteAddr(), s.timeout)
	if s.timeout != 0 {
		err := conn.SetReadDeadline(time.Now().Add(s.timeout))
		if err != nil {
			return nil, err
		}
	}
	n, err := conn.Read(frame.buffer)
	if err != nil {
		return nil, err
	}
	Log("DataGramMessage.Receive: %s %x", conn.RemoteAddr(), frame.buffer)
	if n <= 4 {
		return nil, NewErr("%s: small frame", conn.RemoteAddr())
	}
	if !bytes.Equal(frame.buffer[:2], MAGIC[:2]) {
		return nil, NewErr("%s: wrong magic", conn.RemoteAddr())
	}
	size := int(binary.BigEndian.Uint16(frame.buffer[2:4]))
	if size > max || size < min {
		return nil, NewErr("%s: wrong size %d", conn.RemoteAddr(), size)
	}
	tmp := frame.buffer[4 : 4+size]
	if s.block != nil {
		s.block.Decrypt(tmp, tmp)
	}
	frame.size = size
	frame.frame = tmp
	return frame, nil
}
