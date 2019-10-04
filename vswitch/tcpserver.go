package vswitch

import (
	"net"

	"github.com/lightstar-dev/openlan-go/libol"
)

type TcpServer struct {
	Addr string

	listener   *net.TCPListener
	maxClient  int
	clients    map[*libol.TcpClient]bool
	onClients  chan *libol.TcpClient
	offClients chan *libol.TcpClient
}

func NewTcpServer(c *Config) (this *TcpServer) {
	this = &TcpServer{
		Addr:       c.TcpListen,
		listener:   nil,
		maxClient:  1024,
		clients:    make(map[*libol.TcpClient]bool, 1024),
		onClients:  make(chan *libol.TcpClient, 4),
		offClients: make(chan *libol.TcpClient, 8),
	}

	if err := this.Listen(); err != nil {
		libol.Debug("NewTcpServer %s\n", err)
	}

	return
}

func (this *TcpServer) Listen() error {
	libol.Debug("TcpServer.Start %s\n", this.Addr)

	laddr, err := net.ResolveTCPAddr("tcp", this.Addr)
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		libol.Info("TcpServer.Listen: %s", err)
		this.listener = nil
		return err
	}
	this.listener = listener
	return nil
}

func (this *TcpServer) Close() {
	if this.listener != nil {
		this.listener.Close()
		libol.Info("TcpServer.Close: %s", this.Addr)
		this.listener = nil
	}
}

func (this *TcpServer) GoAccept() {
	libol.Debug("TcpServer.GoAccept")
	if this.listener == nil {
		libol.Error("TcpServer.GoAccept: invalid listener")
	}

	defer this.Close()
	for {
		conn, err := this.listener.AcceptTCP()
		if err != nil {
			libol.Error("TcpServer.GoAccept: %s", err)
			return
		}

		this.onClients <- libol.NewTcpClientFromConn(conn)
	}

	return
}

func (this *TcpServer) GoLoop(onClient func(*libol.TcpClient) error,
	onRecv func(*libol.TcpClient, []byte) error,
	onClose func(*libol.TcpClient) error) {
	libol.Debug("TcpServer.GoLoop")
	defer this.Close()
	for {
		select {
		case client := <-this.onClients:
			libol.Debug("TcpServer.addClient %s", client.Addr)
			if onClient != nil {
				onClient(client)
			}
			this.clients[client] = true
			go this.GoRecv(client, onRecv)
		case client := <-this.offClients:
			if ok := this.clients[client]; ok {
				libol.Debug("TcpServer.delClient %s", client.Addr)
				if onClose != nil {
					onClose(client)
				}
				client.Close()
				delete(this.clients, client)
			}
		}
	}
}

func (this *TcpServer) GoRecv(client *libol.TcpClient, onRecv func(*libol.TcpClient, []byte) error) {
	libol.Debug("TcpServer.GoRecv: %s", client.Addr)
	for {
		data := make([]byte, 4096)
		length, err := client.RecvMsg(data)
		if err != nil {
			this.offClients <- client
			break
		}

		if length > 0 {
			libol.Debug("TcpServer.GoRecv: length: %d ", length)
			libol.Debug("TcpServer.GoRecv: data  : % x", data[:length])
			onRecv(client, data[:length])
		}
	}
}