package libol

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"net"
	"sync"
	"time"
)

const (
	CLINIT       = 0x00
	CLCONNECTED  = 0x01
	CLUNAUTH     = 0x02
	CLAUEHED     = 0x03
	CLCONNECTING = 0x04
	CLTERMINAL   = 0x05
	CLCLOSED     = 0x06
)

type TcpClient struct {
	conn        net.Conn
	maxSize     int
	minSize     int
	onConnected func(*TcpClient) error
	lock        sync.RWMutex

	TxOkay  uint64
	RxOkay  uint64
	TxError uint64
	Dropped uint64
	Status  uint8
	Addr    string
	NewTime int64
	TlsConf *tls.Config
}

func NewTcpClient(addr string, config *tls.Config) (t *TcpClient) {
	t = &TcpClient{
		Addr:        addr,
		conn:        nil,
		maxSize:     1514,
		minSize:     15,
		TxOkay:      0,
		RxOkay:      0,
		TxError:     0,
		Dropped:     0,
		Status:      CLINIT,
		onConnected: nil,
		NewTime:     time.Now().Unix(),
		TlsConf:     config,
	}

	return
}

func NewTcpClientFromConn(conn net.Conn) (t *TcpClient) {
	t = &TcpClient{
		Addr:    conn.RemoteAddr().String(),
		conn:    conn,
		maxSize: 1514,
		minSize: 15,
		NewTime: time.Now().Unix(),
	}

	return
}

func (t *TcpClient) LocalAddr() string {
	return t.conn.LocalAddr().String()
}

func (t *TcpClient) Connect() (err error) {
	if t.conn != nil || t.GetStatus() == CLTERMINAL || t.GetStatus() == CLUNAUTH {
		return nil
	}

	Info("TcpClient.Connect %s,%p", t.Addr, t.TlsConf)

	t.SetStatus(CLCONNECTING)
	if t.TlsConf != nil {
		t.conn, err = tls.Dial("tcp", t.Addr, t.TlsConf)
	} else {
		t.conn, err = net.Dial("tcp", t.Addr)
	}
	if err != nil {
		t.conn = nil
		return err
	}
	t.SetStatus(CLCONNECTED)
	if t.onConnected != nil {
		t.onConnected(t)
	}

	return nil
}

func (t *TcpClient) OnConnected(on func(*TcpClient) error) {
	t.onConnected = on
}

func (t *TcpClient) Close() {
	if t.conn != nil {
		if t.GetStatus() != CLTERMINAL {
			t.SetStatus(CLCLOSED)
		}

		Info("TcpClient.Close %s", t.Addr)
		t.conn.Close()
		t.conn = nil
	}
}

func (t *TcpClient) ReadFull(buffer []byte) error {
	Debug("TcpClient.ReadFull %d", len(buffer))

	offset := 0
	left := len(buffer)
	for left > 0 {
		tmp := make([]byte, left)
		n, err := t.conn.Read(tmp)
		if err != nil {
			return err
		}
		copy(buffer[offset:], tmp)
		offset += n
		left -= n
	}

	Debug("TcpClient.ReadFull Data: % x", buffer)

	return nil
}

func (t *TcpClient) WriteFull(buffer []byte) error {
	offset := 0
	size := len(buffer)
	left := size - offset

	Debug("TcpClient.WriteFull %d", size)
	Debug("TcpClient.WriteFull Data: % x", buffer)

	for left > 0 {
		tmp := buffer[offset:]
		Debug("TcpClient.WriteFull tmp %d", len(tmp))
		n, err := t.conn.Write(tmp)
		if err != nil {
			return err
		}
		offset += n
		left = size - offset
	}
	return nil
}

func (t *TcpClient) WriteMsg(data []byte) error {
	if err := t.Connect(); err != nil {
		return err
	}

	size := len(data)
	buf := make([]byte, HSIZE+size)
	copy(buf[0:2], MAGIC)
	binary.BigEndian.PutUint16(buf[2:4], uint16(size))
	copy(buf[HSIZE:], data)

	if err := t.WriteFull(buf); err != nil {
		t.TxError++
		return err
	}

	t.TxOkay += uint64(size)

	return nil
}

func (t *TcpClient) ReadMsg(data []byte) (int, error) {
	Debug("TcpClient.ReadMsg %s", t)

	if !t.IsOk() {
		return -1, NewErr("%s: not okay", t)
	}

	b := make([]byte, HSIZE+t.maxSize)
	h := b[:HSIZE]
	if err := t.ReadFull(h); err != nil {
		return -1, err
	}
	if !bytes.Equal(h[0:2], MAGIC) {
		return -1, NewErr("%s: wrong magic", t)
	}

	size := binary.BigEndian.Uint16(h[2:4])
	if int(size) > t.maxSize || int(size) < t.minSize {
		return -1, NewErr("%s: wrong size(%d)", t, size)
	}
	d := b[HSIZE : HSIZE+size]
	if err := t.ReadFull(d); err != nil {
		return -1, err
	}

	copy(data, d)
	t.RxOkay += uint64(size)

	return len(d), nil
}

func (t *TcpClient) GetMaxSize() int {
	return t.maxSize
}

func (t *TcpClient) SetMaxSize(value int) {
	t.maxSize = value
}

func (t *TcpClient) GetMinSize() int {
	return t.minSize
}

func (t *TcpClient) IsOk() bool {
	return t.conn != nil
}

func (t *TcpClient) IsTerminal() bool {
	return t.GetStatus() == CLTERMINAL
}

func (t *TcpClient) IsInitialized() bool {
	return t.GetStatus() == CLINIT
}

func (t *TcpClient) WriteReq(action string, body string) error {
	data := EncodeRequest(action, body)
	Debug("TcpClient.WriteReq %d %s", len(data), data[6:])

	if err := t.WriteMsg(data); err != nil {
		return err
	}
	return nil
}

func (t *TcpClient) WriteResp(action string, body string) error {
	data := EncodeReply(action, body)
	Debug("TcpClient.WriteResp %d %s", len(data), data[6:])

	if err := t.WriteMsg(data); err != nil {
		return err
	}
	return nil
}

func (t *TcpClient) GetState() string {
	switch t.GetStatus() {
	case CLINIT:
		return "initialized"
	case CLCONNECTED:
		return "connected"
	case CLUNAUTH:
		return "unauthenticated"
	case CLAUEHED:
		return "authenticated"
	case CLCLOSED:
		return "closed"
	case CLCONNECTING:
		return "connecting"
	case CLTERMINAL:
		return "terminal"
	}
	return ""
}

func (t *TcpClient) UpTime() int64 {
	return time.Now().Unix() - t.NewTime
}

func (t *TcpClient) String() string {
	return t.Addr
}

func (t *TcpClient) Terminal() {
	t.SetStatus(CLTERMINAL)
	t.Close()
}

func (t *TcpClient) GetStatus() uint8 {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.Status
}

func (t *TcpClient) SetStatus(v uint8) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.Status = v
}
