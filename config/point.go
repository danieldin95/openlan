package config

import (
	"flag"
	"fmt"
	"strings"

	"github.com/lightstar-dev/openlan-go/libol"
)

type Point struct {
	Addr     string `json:"VsAddr"`
	Auth     string `json:"VsAuth"`
	Tls      bool   `json:"VsTls"`
	Verbose  int    `json:"Verbose"`
	IfMtu    int    `json:"IfMtu"`
	IfAddr   string `json:"IfAddr"`
	BrName   string `json:"IfBridge"`
	IfTun    bool   `json:"IfTun"`
	IfEthSrc string `json:"IfEthSrc"`
	IfEthDst string `json:"IfEthDst"`
	LogFile  string `json:"LogFile"`

	SaveFile string `json:"-"`
	name     string
	password string
}

var PointDefault = Point{
	Addr:     "openlan.net",
	Auth:     "hi:hi@123$",
	Verbose:  libol.INFO,
	IfMtu:    1518,
	IfAddr:   "",
	IfTun:    false,
	BrName:   "",
	SaveFile: ".point.json",
	name:     "",
	password: "",
	IfEthDst: "2e:4b:f0:b7:6d:ba",
	IfEthSrc: "",
	LogFile:  ".point.error",
}

func NewPoint() (c *Point) {
	c = &Point{
		LogFile: PointDefault.LogFile,
	}

	flag.StringVar(&c.Addr, "vs:addr", PointDefault.Addr, "the server connect to")
	flag.StringVar(&c.Auth, "vs:auth", PointDefault.Auth, "the auth login to")
	flag.BoolVar(&c.Tls, "vs:tls", PointDefault.Tls, "Enable TLS to decrypt")
	flag.IntVar(&c.Verbose, "verbose", PointDefault.Verbose, "open verbose")
	flag.IntVar(&c.IfMtu, "if:mtu", PointDefault.IfMtu, "the interface MTU include ethernet")
	flag.StringVar(&c.IfAddr, "if:addr", PointDefault.IfAddr, "the interface address")
	flag.StringVar(&c.BrName, "if:br", PointDefault.BrName, "the bridge name")
	flag.BoolVar(&c.IfTun, "if:tun", PointDefault.IfTun, "using tun device as interface, otherwise tap")
	flag.StringVar(&c.IfEthDst, "if:ethdst", PointDefault.IfEthDst, "ethernet destination for tun device")
	flag.StringVar(&c.IfEthSrc, "if:ethsrc", PointDefault.IfEthSrc, "ethernet source for tun device")
	flag.StringVar(&c.SaveFile, "conf", PointDefault.SaveFile, "The configuration file")

	flag.Parse()
	if err := c.Load(); err != nil {
		libol.Error("NewPoint.load %s", err)
	}
	c.Default()

	libol.Init(c.LogFile, c.Verbose)
	c.Save(fmt.Sprintf("%s.cur", c.SaveFile))

	str, err := libol.Marshal(c, false)
	if err != nil {
		libol.Error("NewPoint.json error: %s", err)
	}
	libol.Debug("NewPoint.json: %s", str)

	return
}

func (c *Point) Right() {
	if c.Auth != "" {
		values := strings.Split(c.Auth, ":")
		c.name = values[0]
		if len(values) > 1 {
			c.password = values[1]
		}
	}
	RightAddr(&c.Addr, 10002)
}

func (c *Point) Default() {
	c.Right()

	//reset zero value to default
	if c.Addr == "" {
		c.Addr = PointDefault.Addr
	}
	if c.Auth == "" {
		c.Auth = PointDefault.Auth
	}
	if c.IfMtu == 0 {
		c.IfMtu = PointDefault.IfMtu
	}
	if c.IfAddr == "" {
		c.IfAddr = PointDefault.IfAddr
	}
}

func (c *Point) Name() string {
	return c.name
}

func (c *Point) Password() string {
	return c.password
}

func (c *Point) Save(file string) error {
	if file == "" {
		file = c.SaveFile
	}

	return libol.MarshalSave(c, file, true)
}

func (c *Point) Load() error {
	if err := libol.UnmarshalLoad(c, c.SaveFile); err != nil {
		return err
	}

	return nil
}

func init() {
	PointDefault.Right()
}